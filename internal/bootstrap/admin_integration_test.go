package bootstrap

import (
 "context"
 "os"
 "testing"
 "time"

 "backend/internal/password"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Set WRAPLINK_TEST_DATABASE_URL to a disposable database with all migrations applied.
// This test creates an administrator and must never run against production.
func TestBootstrapAdminPostgresIntegration(t *testing.T) {
 dsn:=os.Getenv("WRAPLINK_TEST_DATABASE_URL")
 if dsn=="" {t.Skip("WRAPLINK_TEST_DATABASE_URL not set")}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
 defer cancel()
 db,err:=pgxpool.New(ctx,dsn)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 email:="bootstrap-integration-"+time.Now().UTC().Format("20060102150405.000000000")+"@example.invalid"
 original:="bootstrap-test-original-password-2026"
 rotated:="bootstrap-test-rotated-password-2026"
 t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_EMAIL",email)
 t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD",original)
 var id string
 defer func(){
  if id!="" {_,_=db.Exec(context.Background(),"DELETE FROM users WHERE id=$1",id)}
 }()
 if err:=EnsureDefaultAdmin(ctx,db,nil);err!=nil {t.Fatalf("create administrator: %v",err)}
 if err:=db.QueryRow(ctx,"SELECT id::text FROM users WHERE email=$1",email).Scan(&id);err!=nil {t.Fatal(err)}
 var hash string
 if err:=db.QueryRow(ctx,"SELECT password_hash FROM user_credentials WHERE user_id=$1",id).Scan(&hash);err!=nil {t.Fatal(err)}
 if !password.Verify(original,hash){t.Fatal("created password hash does not verify")}
 if err:=EnsureDefaultAdmin(ctx,db,nil);err!=nil {t.Fatalf("idempotent bootstrap: %v",err)}
 var unchanged string
 if err:=db.QueryRow(ctx,"SELECT password_hash FROM user_credentials WHERE user_id=$1",id).Scan(&unchanged);err!=nil {t.Fatal(err)}
 if hash!=unchanged {t.Fatal("unchanged password was rehashed")}
 t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD",rotated)
 if err:=EnsureDefaultAdmin(ctx,db,nil);err!=nil {t.Fatalf("rotate password: %v",err)}
 var changed string
 if err:=db.QueryRow(ctx,"SELECT password_hash FROM user_credentials WHERE user_id=$1",id).Scan(&changed);err!=nil {t.Fatal(err)}
 if !password.Verify(rotated,changed)||password.Verify(original,changed){t.Fatal("password rotation failed")}
 var grants int
 if err:=db.QueryRow(ctx,`SELECT
  (SELECT count(*) FROM access_admins WHERE user_id=$1)+
  (SELECT count(*) FROM payment_review_admins WHERE user_id=$1)+
  (SELECT count(*) FROM discount_admins WHERE user_id=$1)`,id).Scan(&grants);err!=nil {t.Fatal(err)}
 if grants!=3 {t.Fatalf("expected three administrator grants; got %d",grants)}
 // A colliding ordinary account must not gain privileges.
 ordinaryEmail:="ordinary-"+time.Now().UTC().Format("20060102150405.000000000")+"@example.invalid"
 var ordinaryID string
 if err:=db.QueryRow(ctx,"INSERT INTO users(email,full_name,account_status,email_verified) VALUES($1,'Bootstrap Collision Test','active',true) RETURNING id::text",ordinaryEmail).Scan(&ordinaryID);err!=nil {t.Fatal(err)}
 defer func(){_,_=db.Exec(context.Background(),"DELETE FROM users WHERE id=$1",ordinaryID)}()
 t.Setenv("WRAPLINK_BOOTSTRAP_ADMIN_EMAIL",ordinaryEmail)
 if err:=EnsureDefaultAdmin(ctx,db,nil);err==nil {t.Fatal("existing ordinary account was not rejected")}
 var elevated bool
 if err:=db.QueryRow(ctx,"SELECT EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1)",ordinaryID).Scan(&elevated);err!=nil {t.Fatal(err)}
 if elevated {t.Fatal("ordinary account was unexpectedly granted administrator privileges")}
}
