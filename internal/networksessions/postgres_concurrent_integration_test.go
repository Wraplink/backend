package networksessions

import (
 "context"
 "errors"
 "fmt"
 "os"
 "sync"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgconn"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Uses committed, uniquely named fixtures so separate PostgreSQL connections
// can race against the real partial unique index. Never run on production.
func TestPostgresConcurrentPendingDisconnects(t *testing.T) {
 dsn:=os.Getenv("TEST_DATABASE_URL")
 if dsn=="" {t.Skip("TEST_DATABASE_URL not configured")}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
 defer cancel()
 db,err:=pgxpool.New(ctx,dsn)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 id:=uuid.New()
 suffix:=id.String()
 agent:="concurrent-agent-"+suffix
 ip:=fmt.Sprintf("198.19.%d.%d",id[0],id[1])
 attempts:=(int(id[2])<<24|int(id[3])<<16|int(id[4])<<8|int(id[5])) & 0x7fffffff
 var userID,planID,subscriptionID,sessionID uuid.UUID
 tx,err:=db.Begin(ctx)
 if err!=nil {t.Fatal(err)}
 defer tx.Rollback(context.Background())
 if err:=tx.QueryRow(ctx,`INSERT INTO users(email,full_name,last_login_ip,failed_login_attempts)
 VALUES($1,'Concurrent Session Test',$2,$3) RETURNING id`,"concurrent-"+suffix+"@example.invalid",ip,attempts).Scan(&userID);err!=nil {t.Fatal(err)}
 if err:=tx.QueryRow(ctx,`INSERT INTO plans(code,name,price_minor,currency,duration_days,data_limit_bytes)
 VALUES($1,'Concurrent Session Test',0,'USD',1,1024) RETURNING id`,"concurrent-"+suffix).Scan(&planID);err!=nil {t.Fatal(err)}
 if err:=tx.QueryRow(ctx,`INSERT INTO user_subscriptions(user_id,plan_id,plan_code,plan_name,price_minor,currency,duration_days,data_limit_bytes,status)
 VALUES($1,$2,$3,'Concurrent Session Test',0,'USD',1,1024,'active') RETURNING id`,userID,planID,"concurrent-"+suffix).Scan(&subscriptionID);err!=nil {t.Fatal(err)}
 if _,err:=tx.Exec(ctx,`INSERT INTO network_agent_health(id) VALUES($1)`,agent);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 if err:=tx.QueryRow(ctx,`INSERT INTO network_sessions(agent_id,agent_session_id,user_id,subscription_id,client_ip,mode,protocol,connected_at,last_activity_at,last_report_at)
 VALUES($1,$2,$3,$4,$5,'sni','tcp',$6,$6,$6) RETURNING id`,agent,"concurrent-"+suffix,userID,subscriptionID,ip,now).Scan(&sessionID);err!=nil {t.Fatal(err)}
 if err:=tx.Commit(ctx);err!=nil {t.Fatal(err)}
 defer func(){
  cleanupCtx,done:=context.WithTimeout(context.Background(),10*time.Second)
  defer done()
  for _,q:=range []struct{sql string;arg any}{
   {"DELETE FROM network_disconnect_commands WHERE network_session_id=$1",sessionID},
   {"DELETE FROM network_sessions WHERE id=$1",sessionID},
   {"DELETE FROM network_agent_health WHERE id=$1",agent},
   {"DELETE FROM user_subscriptions WHERE id=$1",subscriptionID},
   {"DELETE FROM plans WHERE id=$1",planID},
   {"DELETE FROM users WHERE id=$1",userID},
  } {
   if _,err:=db.Exec(cleanupCtx,q.sql,q.arg);err!=nil {t.Errorf("fixture cleanup: %v",err)}
  }
 }()
 const workers=12
 var wg sync.WaitGroup
 outcomes:=make(chan error,workers)
 start:=make(chan struct{})
 for i:=0;i<workers;i++ {
  wg.Add(1)
  go func(i int){
   defer wg.Done()
   <-start
   _,err:=db.Exec(ctx,`INSERT INTO network_disconnect_commands(network_session_id,agent_id,actor_user_id,reason,idempotency_key)
 VALUES($1,$2,$3,'concurrent test',$4)`,sessionID,agent,userID,fmt.Sprintf("%s-%d",suffix,i))
   outcomes<-err
  }(i)
 }
 close(start)
 wg.Wait()
 close(outcomes)
 inserted,conflicts:=0,0
 for err:=range outcomes {
  if err==nil {inserted++;continue}
  var pgErr *pgconn.PgError
  if errors.As(err,&pgErr) && pgErr.Code=="23505" {conflicts++;continue}
  t.Fatalf("unexpected concurrent insertion error: %v",err)
 }
 if inserted!=1 || conflicts!=workers-1 {
  t.Fatalf("expected one pending command and %d conflicts; got %d inserted, %d conflicts",workers-1,inserted,conflicts)
 }
 var pending int
 if err:=db.QueryRow(ctx,`SELECT count(*) FROM network_disconnect_commands WHERE network_session_id=$1 AND state='pending'`,sessionID).Scan(&pending);err!=nil {t.Fatal(err)}
 if pending!=1 {t.Fatalf("expected exactly one persisted pending command, got %d",pending)}
}
