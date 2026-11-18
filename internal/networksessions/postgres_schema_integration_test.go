package networksessions

import (
 "context"
 "os"
 "testing"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

// Requires the migrated PostgreSQL service provided by the backend CI job.
func TestNetworkSessionsMigrationConstraints(t *testing.T) {
 url:=os.Getenv("TEST_DATABASE_URL")
 if url=="" {t.Skip("TEST_DATABASE_URL is not configured")}
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second)
 defer cancel()
 pool,err:=pgxpool.New(ctx,url)
 if err!=nil {t.Fatal(err)}
 defer pool.Close()

 constraints:=[]string{
  "network_sessions_identity_unique",
  "network_sessions_activity_order",
  "network_sessions_sni_tcp",
  "network_sessions_disconnected_timestamp",
  "network_disconnect_idempotency_unique",
 }
 for _,name:=range constraints {
  t.Run(name,func(t *testing.T){
   var exists bool
   err:=pool.QueryRow(ctx,`
    SELECT EXISTS (
     SELECT 1 FROM pg_constraint c
     JOIN pg_class r ON r.oid=c.conrelid
     WHERE r.relname IN ('network_sessions','network_disconnect_commands')
      AND c.conname=$1
    )`,name).Scan(&exists)
   if err!=nil {t.Fatal(err)}
   if !exists {t.Fatalf("migration constraint %q is missing",name)}
  })
 }
 var indexExists bool
 err=pool.QueryRow(ctx,`
  SELECT EXISTS (
   SELECT 1 FROM pg_indexes
   WHERE tablename='network_disconnect_commands'
    AND indexname='idx_network_disconnect_one_pending_per_session'
    AND indexdef ILIKE '%UNIQUE%'
    AND indexdef ILIKE '%state = ''pending''%'
  )`).Scan(&indexExists)
 if err!=nil {t.Fatal(err)}
 if !indexExists {t.Fatal("unique pending-disconnect index missing or not partial")}
}
