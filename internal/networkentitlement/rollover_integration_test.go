package networkentitlement

import (
    "context"
    "os"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

// This test is opt-in and must only run against a disposable, migrated database.
// It verifies that snapshot reconciliation preserves the original entitlement
// for historical accounting while moving access to a promoted subscription.
func TestReservedEntitlementRolloverPostgres(t *testing.T) {
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" { t.Skip("set TEST_DATABASE_URL to a disposable migrated PostgreSQL database") }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    pool, err := pgxpool.New(ctx, url)
    if err != nil { t.Fatal(err) }
    defer pool.Close()

    userID, planID := uuid.New(), uuid.New()
    oldID, nextID := uuid.New(), uuid.New()
    ip := "198.51.100.93"
    now := time.Now().UTC()
    _, err = pool.Exec(ctx, `INSERT INTO users
        (id,failed_login_attempts,last_login_ip,email,full_name)
        VALUES ($1,$2,$3::inet,$4,'Rollover integration fixture')`,
        userID, int(now.UnixNano()%2000000000), "198.51.100.94", userID.String()+"@example.invalid")
    if err != nil { t.Fatal(err) }
    defer func(){ _,_ = pool.Exec(context.Background(),"DELETE FROM users WHERE id=$1",userID) }()
    _, err = pool.Exec(ctx, `INSERT INTO plans
        (id,code,name,price_minor,currency,duration_days,data_limit_bytes)
        VALUES ($1,$2,'Rollover plan',0,'USD',30,1000)`,planID,"rollover-"+planID.String())
    if err != nil { t.Fatal(err) }
    defer func(){ _,_ = pool.Exec(context.Background(),"DELETE FROM plans WHERE id=$1",planID) }()

    _, err = pool.Exec(ctx, `INSERT INTO user_subscriptions
        (id,user_id,plan_id,plan_code,plan_name,price_minor,currency,duration_days,
         data_limit_bytes,used_bytes,status,starts_at,expires_at)
        VALUES ($1,$3,$4,'rollover','Rollover plan',0,'USD',30,1000,1000,'active',$5,$6),
               ($2,$3,$4,'rollover','Rollover plan',0,'USD',30,1000,0,'confirmed',NULL,NULL)`,
        oldID,nextID,userID,planID,now.Add(-time.Hour),now.Add(time.Hour))
    if err != nil { t.Fatal(err) }
    _, err = pool.Exec(ctx, `INSERT INTO network_entitlements
        (user_id,subscription_id,client_ip,status,activated_at,last_activity_at)
        VALUES ($1,$2,$3::inet,'active',$4,$4)`,userID,oldID,ip,now.Add(-time.Minute))
    if err != nil { t.Fatal(err) }

    repo := NewRepository(pool)
    if _, err := repo.Snapshot(ctx); err != nil { t.Fatal(err) }
    var activeCount, oldRevokedCount int
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM network_entitlements
        WHERE user_id=$1 AND subscription_id=$2 AND status='active'`,userID,nextID).Scan(&activeCount); err != nil { t.Fatal(err) }
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM network_entitlements
        WHERE user_id=$1 AND subscription_id=$2 AND status='revoked' AND revoked_at IS NOT NULL`,userID,oldID).Scan(&oldRevokedCount); err != nil { t.Fatal(err) }
    if activeCount != 1 || oldRevokedCount != 1 {
        t.Fatalf("rollover active=%d old revoked=%d; want 1 each",activeCount,oldRevokedCount)
    }
    if _, err := repo.Snapshot(ctx); err != nil { t.Fatal(err) }
    if err := pool.QueryRow(ctx, `SELECT count(*) FROM network_entitlements
        WHERE user_id=$1 AND status='active'`,userID).Scan(&activeCount); err != nil { t.Fatal(err) }
    if activeCount != 1 { t.Fatalf("repeated snapshot created %d active entitlements",activeCount) }
}
