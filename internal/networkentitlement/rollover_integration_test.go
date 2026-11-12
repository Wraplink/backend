package networkentitlement

import (
    "context"
    "errors"
    "os"
    "sync"
    "backend/internal/networkaccess"
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
    // Exercise concurrent reconciliation by snapshot synchronization and
    // user-facing connection-status reads. Both must converge to one active
    // entitlement rather than inserting duplicate replacements.
    var wg sync.WaitGroup
    failures := make(chan error, 8)
    for i := 0; i < 4; i++ {
        wg.Add(2)
        go func() {
            defer wg.Done()
            _, err := repo.Snapshot(ctx)
            failures <- err
        }()
        go func() {
            defer wg.Done()
            _, err := repo.Current(ctx, userID)
            failures <- err
        }()
    }
    wg.Wait()
    close(failures)
    for err := range failures {
        if err != nil { t.Fatalf("concurrent rollover: %v", err) }
    }
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
    // A delayed edge report entirely within the old entitlement interval
    // must still be charged to the historical subscription after rollover.
    edge := networkaccess.EdgeUsageReport{
        AgentID: "rollover-"+uuid.NewString(), Sequence: 1, ClientIP: ip,
        UploadedBytes: 25, DownloadedBytes: 75,
        StartedAt: now.Add(-50*time.Second), EndedAt: now.Add(-40*time.Second),
    }
    usage := networkaccess.NewService(networkaccess.NewRepository(pool),
        networkaccess.ConnectionDetails{}, nil, 0)
    result, err := usage.IngestEdgeUsage(ctx, edge)
    if err != nil { t.Fatalf("historical edge report after rollover: %v", err) }
    if result.SubscriptionID != oldID || !result.Applied {
        t.Fatalf("historical report subscription=%s applied=%v; want old subscription", result.SubscriptionID, result.Applied)
    }
    var oldUsage int64
    if err := pool.QueryRow(ctx, "SELECT used_bytes FROM user_subscriptions WHERE id=$1",oldID).Scan(&oldUsage); err != nil { t.Fatal(err) }
    if oldUsage != 1100 { t.Fatalf("historical usage=%d, want 1100",oldUsage) }
    // Concurrent snapshots and a second delayed edge report must not
    // reassign historical traffic to the newly activated plan.
    edge2 := edge
    edge2.Sequence = 2
    edge2.UploadedBytes = 40
    edge2.DownloadedBytes = 60
    var concurrent sync.WaitGroup
    concurrentErrors := make(chan error, 5)
    concurrent.Add(1)
    go func() {
        defer concurrent.Done()
        _, err := usage.IngestEdgeUsage(ctx, edge2)
        concurrentErrors <- err
    }()
    for i := 0; i < 4; i++ {
        concurrent.Add(1)
        go func() {
            defer concurrent.Done()
            _, err := repo.Snapshot(ctx)
            concurrentErrors <- err
        }()
    }
    concurrent.Wait()
    close(concurrentErrors)
    for err := range concurrentErrors {
        if err != nil { t.Fatalf("concurrent edge report and snapshot: %v", err) }
    }
    var newUsage int64
    if err := pool.QueryRow(ctx, "SELECT used_bytes FROM user_subscriptions WHERE id=$1",nextID).Scan(&newUsage); err != nil { t.Fatal(err) }
    if newUsage != 0 { t.Fatalf("new subscription charged historical traffic: %d",newUsage) }
    if err := pool.QueryRow(ctx, "SELECT used_bytes FROM user_subscriptions WHERE id=$1",oldID).Scan(&oldUsage); err != nil { t.Fatal(err) }
    if oldUsage != 1200 { t.Fatalf("historical usage=%d, want 1200",oldUsage) }
    // A report crossing the entitlement switch cannot be attributed
    // accurately from aggregate bytes alone. Reject it rather than
    // charging either plan incorrectly; edge must report split intervals.
    var switchAt time.Time
    if err := pool.QueryRow(ctx, `SELECT activated_at FROM network_entitlements
        WHERE user_id=$1 AND subscription_id=$2 AND status='active'`,
        userID,nextID).Scan(&switchAt); err != nil { t.Fatal(err) }
    crossing := edge
    crossing.Sequence = 3
    crossing.StartedAt = switchAt.Add(-time.Second)
    crossing.EndedAt = switchAt.Add(time.Second)
    _, err = usage.IngestEdgeUsage(ctx, crossing)
    if !errors.Is(err, networkaccess.ErrSubscriptionNotFound) {
        t.Fatalf("cross-boundary report error=%v, want ErrSubscriptionNotFound",err)
    }
    if err := pool.QueryRow(ctx, "SELECT used_bytes FROM user_subscriptions WHERE id=$1",nextID).Scan(&newUsage); err != nil { t.Fatal(err) }
    if newUsage != 0 { t.Fatalf("cross-boundary report charged new subscription: %d",newUsage) }
    if err := pool.QueryRow(ctx, "SELECT used_bytes FROM user_subscriptions WHERE id=$1",oldID).Scan(&oldUsage); err != nil { t.Fatal(err) }
    if oldUsage != 1200 { t.Fatalf("cross-boundary report charged old subscription: %d",oldUsage) }
}
