package networksessions

import (
 "context"
 "errors"
 "fmt"
 "os"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgconn"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Exercises the real partial unique index in a rollback-only transaction.
// CI applies migrations and provides TEST_DATABASE_URL.
func TestPostgresOnlyOnePendingDisconnectPerSession(t *testing.T) {
 dsn:=os.Getenv("TEST_DATABASE_URL")
 if dsn=="" {t.Skip("TEST_DATABASE_URL not set")}
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second)
 defer cancel()
 db,err:=pgxpool.New(ctx,dsn)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 tx,err:=db.Begin(ctx)
 if err!=nil {t.Fatal(err)}
 defer tx.Rollback(context.Background())

 suffix:=uuid.NewString()
 agentID:="test-agent-"+suffix
 email:="network-session-"+suffix+"@example.invalid"
 planCode:="network-test-"+suffix
 var userID,planID,subscriptionID,sessionID uuid.UUID
 // Use a unique nonzero login-attempt count and IP because the original
 // users migration enforces uniqueness on both columns.
 attemptCount:=int(time.Now().UnixNano()%1000000000)+100000000
 if err:=tx.QueryRow(ctx,`INSERT INTO users(email,full_name,last_login_ip,failed_login_attempts)
 VALUES($1,'Network Session Integration','198.18.0.1',$2) RETURNING id`,email,attemptCount).Scan(&userID);err!=nil {t.Fatal(err)}
 if err:=tx.QueryRow(ctx,`INSERT INTO plans(code,name,price_minor,currency,duration_days,data_limit_bytes)
 VALUES($1,'Network Session Integration',0,'USD',1,1024) RETURNING id`,planCode).Scan(&planID);err!=nil {t.Fatal(err)}
 if err:=tx.QueryRow(ctx,`INSERT INTO user_subscriptions(user_id,plan_id,plan_code,plan_name,price_minor,currency,duration_days,data_limit_bytes,status)
 VALUES($1,$2,$3,'Network Session Integration',0,'USD',1,1024,'active') RETURNING id`,userID,planID,planCode).Scan(&subscriptionID);err!=nil {t.Fatal(err)}
 if _,err:=tx.Exec(ctx,`INSERT INTO network_agent_health(id) VALUES($1)`,agentID);err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 if err:=tx.QueryRow(ctx,`INSERT INTO network_sessions(agent_id,agent_session_id,user_id,subscription_id,client_ip,mode,protocol,connected_at,last_activity_at,last_report_at)
 VALUES($1,$2,$3,$4,'192.0.2.10','sni','tcp',$5,$5,$5) RETURNING id`,agentID,"test-session-"+suffix,userID,subscriptionID,now).Scan(&sessionID);err!=nil {t.Fatal(err)}
 insert:=func(key string)error {
  _,err:=tx.Exec(ctx,`INSERT INTO network_disconnect_commands(network_session_id,agent_id,actor_user_id,reason,idempotency_key)
 VALUES($1,$2,$3,'integration test',$4)`,sessionID,agentID,userID,key)
  return err
 }
 // A second report for the same agent-local session must not create a
 // second database identity, even when the reporter repeats its payload.
 if _,err:=tx.Exec(ctx,"SAVEPOINT duplicate_session");err!=nil {t.Fatal(err)}
 _,err=tx.Exec(ctx,`INSERT INTO network_sessions(agent_id,agent_session_id,user_id,subscription_id,client_ip,mode,protocol,connected_at,last_activity_at,last_report_at)
 VALUES($1,$2,$3,$4,'192.0.2.10','sni','tcp',$5,$5,$5)`,agentID,"test-session-"+suffix,userID,subscriptionID,now)
 var duplicateSessionErr *pgconn.PgError
 if !errors.As(err,&duplicateSessionErr) || duplicateSessionErr.Code!="23505" {
  t.Fatalf("expected unique session identity violation, got %v",err)
 }
 if _,err:=tx.Exec(ctx,"ROLLBACK TO SAVEPOINT duplicate_session");err!=nil {t.Fatal(err)}
 if err:=insert("first-"+suffix);err!=nil {t.Fatal(err)}
 if _,err:=tx.Exec(ctx,"SAVEPOINT duplicate_pending");err!=nil {t.Fatal(err)}
 err=insert("second-"+suffix)
 var pgErr *pgconn.PgError
 if !errors.As(err,&pgErr) || pgErr.Code!="23505" {
  t.Fatalf("expected unique violation for concurrent pending command, got %v",err)
 }
 if _,err:=tx.Exec(ctx,"ROLLBACK TO SAVEPOINT duplicate_pending");err!=nil {t.Fatal(err)}
 if _,err:=tx.Exec(ctx,`UPDATE network_disconnect_commands SET state='succeeded' WHERE network_session_id=$1 AND state='pending'`,sessionID);err!=nil {t.Fatal(err)}
 if err:=insert("second-"+suffix);err!=nil {t.Fatalf("new pending command after finalization: %v",err)}
 var pending,total int
 if err:=tx.QueryRow(ctx,`SELECT count(*) FILTER (WHERE state='pending'),count(*) FROM network_disconnect_commands WHERE network_session_id=$1`,sessionID).Scan(&pending,&total);err!=nil {t.Fatal(err)}
 if pending!=1 || total!=2 {t.Fatal(fmt.Sprintf("expected 1 pending of 2 commands, got %d of %d",pending,total))}
 // Explicitly roll back and verify no fixture escaped the transaction.
 if err:=tx.Rollback(ctx);err!=nil {t.Fatal(err)}
 var persisted int
 if err:=db.QueryRow(ctx,`SELECT count(*) FROM network_agent_health WHERE id=$1`,agentID).Scan(&persisted);err!=nil {t.Fatal(err)}
 if persisted!=0 {t.Fatal("rolled-back agent fixture persisted")}
 if err:=db.QueryRow(ctx,`SELECT count(*) FROM network_disconnect_commands WHERE network_session_id=$1`,sessionID).Scan(&persisted);err!=nil {t.Fatal(err)}
 if persisted!=0 {t.Fatal("rolled-back disconnect commands persisted")}
}
