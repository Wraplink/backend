package bootstrap

import (
 "context"
 "errors"
 "fmt"
 "log/slog"
 "net/mail"
 "os"
 "strings"

 "backend/internal/password"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

// EnsureDefaultAdmin provisions an initial administrator only when explicitly configured.
// Never ships with a known password, and never promotes an existing ordinary user.
func EnsureDefaultAdmin(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) error {
 if logger==nil {logger=slog.Default()}
 email:=strings.ToLower(strings.TrimSpace(os.Getenv("WRAPLINK_BOOTSTRAP_ADMIN_EMAIL")))
 secret:=os.Getenv("WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD")
 if email=="" && secret=="" {return nil}
 if email=="" || secret=="" {return errors.New("bootstrap admin requires both email and password")}
 parsed,err:=mail.ParseAddress(email)
 if err!=nil || parsed.Address!=email || len(email)>254 {return errors.New("invalid bootstrap admin email")}
 if len(secret)<16 || len(secret)>1024 {return errors.New("bootstrap admin password must be 16-1024 characters")}
 hash,err:=password.Hash(secret)
 if err!=nil{return fmt.Errorf("hash bootstrap password: %w",err)}
 tx,err:=db.Begin(ctx)
 if err!=nil{return err}
 defer tx.Rollback(ctx)
 // Serializes multiple app instances attempting the same bootstrap.
 if _,err=tx.Exec(ctx,"SELECT pg_advisory_xact_lock(87822109)");err!=nil{return err}
 var id uuid.UUID
 err=tx.QueryRow(ctx,"SELECT id FROM users WHERE email=$1",email).Scan(&id)
 switch {
 case errors.Is(err,pgx.ErrNoRows):
  err=tx.QueryRow(ctx,`INSERT INTO users(email,full_name,account_status,email_verified)
   VALUES($1,'WrapLink Administrator','active',true) RETURNING id`,email).Scan(&id)
  if err!=nil{return fmt.Errorf("create bootstrap admin: %w",err)}
  if _,err=tx.Exec(ctx,"INSERT INTO user_credentials(user_id,password_hash) VALUES($1,$2)",id,hash);err!=nil{return err}
  for _,table:=range []string{"access_admins","payment_review_admins","discount_admins"} {
   if _,err=tx.Exec(ctx,"INSERT INTO "+table+"(user_id) VALUES($1)",id);err!=nil{return err}
  }
  if _,err=tx.Exec(ctx,"INSERT INTO security_events(user_id,event_type) VALUES($1,'ADMIN_BOOTSTRAPPED')",id);err!=nil{return err}
  if err=tx.Commit(ctx);err!=nil{return err}
  logger.Info("initial administrator created","email",email)
  return nil
 case err!=nil:
  return err
 default:
  var allowed bool
  if err=tx.QueryRow(ctx,"SELECT EXISTS(SELECT 1 FROM access_admins WHERE user_id=$1)",id).Scan(&allowed);err!=nil{return err}
  if !allowed{return errors.New("bootstrap admin email belongs to an existing non-admin user; refusing privilege escalation")}
  logger.Info("bootstrap administrator already exists","email",email)
  return nil
 }
}
