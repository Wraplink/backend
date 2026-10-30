package networkaccess

import (
 "context"
 "errors"
 "os"
 "sync"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Requires a disposable, fully migrated PostgreSQL database.
// Run with TEST_DATABASE_URL set; never point this test at production.
func TestEdgeUsageConcurrentDuplicateDeliveryPostgres(t *testing.T) {
 url := os.Getenv("TEST_DATABASE_URL")
 if url == "" { t.Skip("set TEST_DATABASE_URL to a disposable migrated PostgreSQL database") }
 ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
 defer cancel()
 pool, err := pgxpool.New(ctx, url)
 if err != nil { t.Fatal(err) }
 defer pool.Close()
 userID, planID, subscriptionID := uuid.New(), uuid.New(), uuid.New()
 ip := "198.51.100.91"
 // Unique legacy columns in users require distinct values for each fixture.
 attempt := int(time.Now().UnixNano() % 2000000000)
 _, err = pool.Exec(ctx, `INSERT INTO users
 (id,failed_login_attempts,last_login_ip,email,full_name)
 VALUES ($1,$2,$3::inet,$4,'Usage integration fixture')`,
 userID, -attempt, "198.51.100.92", userID.String()+"@example.invalid")
 if err != nil { t.Fatal(err) }
 defer func(){ _,_ = pool.Exec(context.Background(),"DELETE FROM users WHERE id=$1",userID) }()
 _,err=pool.Exec(ctx,`INSERT INTO plans
 (id,code,name,price_minor,currency,duration_days,data_limit_bytes)
 VALUES ($1,$2,'Integration plan',0,'USD',30,1000000)`,planID,"edge-test-"+planID.String())
 if err!=nil {t.Fatal(err)}
 defer func(){_,_=pool.Exec(context.Background(),"DELETE FROM plans WHERE id=$1",planID)}()
 now:=time.Now().UTC()
 _,err=pool.Exec(ctx,`INSERT INTO user_subscriptions
 (id,user_id,plan_id,plan_code,plan_name,price_minor,currency,duration_days,data_limit_bytes,status,starts_at,expires_at)
 VALUES ($1,$2,$3,'integration','Integration plan',0,'USD',30,1000000,'active',$4,$5)`,
 subscriptionID,userID,planID,now.Add(-time.Hour),now.Add(time.Hour))
 if err!=nil {t.Fatal(err)}
 _,err=pool.Exec(ctx,`INSERT INTO network_entitlements
 (user_id,subscription_id,client_ip,status,activated_at,last_activity_at)
 VALUES ($1,$2,$3::inet,'active',$4,$4)`,userID,subscriptionID,ip,now.Add(-time.Hour))
 if err!=nil {t.Fatal(err)}
 edge:=EdgeUsageReport{AgentID:"integration-"+uuid.NewString(),Sequence:1,ClientIP:ip,
 UploadedBytes:100,DownloadedBytes:200,StartedAt:now.Add(-time.Minute),EndedAt:now}
 service:=&Service{repository:NewRepository(pool)}
 const workers=8
 var wg sync.WaitGroup
 results:=make(chan error,workers)
 applied:=make(chan bool,workers)
 for i:=0;i<workers;i++ {
 wg.Add(1)
 go func(){
 defer wg.Done()
 result,err:=service.IngestEdgeUsage(ctx,edge)
 results<-err
 if err==nil {applied<-result.Applied}
 }()
 }
 wg.Wait()
 close(results);close(applied)
 for err:=range results {if err!=nil {t.Fatalf("concurrent delivery failed: %v",err)}}
 appliedCount:=0
 for value:=range applied {if value {appliedCount++}}
 if appliedCount!=1 {t.Fatalf("applied count=%d, want 1",appliedCount)}
 var used int64
 if err:=pool.QueryRow(ctx,"SELECT used_bytes FROM user_subscriptions WHERE id=$1",subscriptionID).Scan(&used);err!=nil {t.Fatal(err)}
 if used!=300 {t.Fatalf("used bytes=%d, want 300",used)}
 conflicting:=edge
 conflicting.UploadedBytes++
 if _,err:=service.IngestEdgeUsage(ctx,conflicting);!errors.Is(err,ErrReportConflict) {
 t.Fatalf("conflicting replay error=%v, want ErrReportConflict",err)
 }
}
