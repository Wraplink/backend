package networksessions

import (
 "context"
 "github.com/go-chi/chi/v5"
 "crypto/sha256"
 "net/http"
 "net/http/httptest"
 "strings"
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
   {"DELETE FROM network_disconnect_commands WHERE agent_id=$1",agent},
   {"DELETE FROM network_sessions WHERE agent_id=$1",agent},
   {"DELETE FROM network_entitlements WHERE subscription_id=$1",subscriptionID},
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
 // The HTTP report handler uses the same transaction-scoped upsert helper.
 // Prove that a successful first write is undone when a later batch member
 // has no matching entitlement (a failure discovered after validation).
 if _,err:=db.Exec(ctx,`UPDATE user_subscriptions SET starts_at=now()-interval '1 hour', expires_at=now()+interval '1 day' WHERE id=$1`,subscriptionID);err!=nil {t.Fatal(err)}
 if _,err:=db.Exec(ctx,`INSERT INTO network_entitlements(user_id,subscription_id,client_ip,status)
 VALUES($1,$2,$3,'active')`,userID,subscriptionID,ip);err!=nil {t.Fatal(err)}
 // Exercise the real authenticated HTTP handler using the committed
 // entitlement fixture, not only the lower-level repository helper.
 token:="session-report-"+suffix
 digest:=sha256.Sum256([]byte(token))
 if _,err:=db.Exec(ctx,`INSERT INTO network_session_agent_credentials(agent_id,token_sha256,enabled) VALUES($1,$2,true)`,agent,digest[:]);err!=nil {t.Fatal(err)}
 httpNow:=time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
 payload:=fmt.Sprintf(`{"sessions":[{"sessionId":"http-success-%s","clientIp":"%s","mode":"sni","protocol":"tcp","connectedAt":"%s","lastActivityAt":"%s","uploadBytes":0,"downloadBytes":0}]}`,suffix,ip,httpNow,httpNow)
 req:=httptest.NewRequest(http.MethodPost,"/api/v1/internal/network/sessions/report",strings.NewReader(payload))
 req.Header.Set("X-Network-Agent-ID",agent)
 req.Header.Set("Authorization","Bearer "+token)
 recorder:=httptest.NewRecorder()
 (&AgentHandler{Repository:&Repository{DB:db}}).Report(recorder,req)
 if recorder.Code!=http.StatusOK {t.Fatalf("authenticated SNI report: status %d: %s",recorder.Code,recorder.Body.String())}
 var attributedUser,attributedSubscription uuid.UUID
 if err:=db.QueryRow(ctx,`SELECT user_id,subscription_id FROM network_sessions WHERE agent_id=$1 AND agent_session_id=$2`,agent,"http-success-"+suffix).Scan(&attributedUser,&attributedSubscription);err!=nil {t.Fatal(err)}
 if attributedUser!=userID || attributedSubscription!=subscriptionID {t.Fatal("authenticated SNI report attribution mismatch")}
 // Submit an administrator command, poll as the authenticated agent,
 // then acknowledge the exact agent-local SNI session.
 var httpSessionID uuid.UUID
 if err:=db.QueryRow(ctx,`SELECT id FROM network_sessions WHERE agent_id=$1 AND agent_session_id=$2`,agent,"http-success-"+suffix).Scan(&httpSessionID);err!=nil {t.Fatal(err)}
 repository:=&Repository{DB:db}
 command,err:=repository.SubmitDisconnect(ctx,httpSessionID,userID,"integration disconnect","http-"+suffix)
 if err!=nil {t.Fatalf("submit disconnect: %v",err)}
 pollReq:=httptest.NewRequest(http.MethodGet,"/api/v1/internal/network/sessions/commands",nil)
 pollReq.Header.Set("X-Network-Agent-ID",agent)
 pollReq.Header.Set("Authorization","Bearer "+token)
 pollRec:=httptest.NewRecorder()
 (&AgentHandler{Repository:repository}).Commands(pollRec,pollReq)
 if pollRec.Code!=http.StatusOK {t.Fatalf("agent poll: %d: %s",pollRec.Code,pollRec.Body.String())}
 if !strings.Contains(pollRec.Body.String(),command.ID.String()) || !strings.Contains(pollRec.Body.String(),"http-success-"+suffix) {
  t.Fatalf("poll response missing command or target session: %s",pollRec.Body.String())
 }
 ackReq:=httptest.NewRequest(http.MethodPost,"/api/v1/internal/network/sessions/commands/"+command.ID.String()+"/ack",strings.NewReader(`{"status":"succeeded"}`))
 ackReq.Header.Set("X-Network-Agent-ID",agent)
 ackReq.Header.Set("Authorization","Bearer "+token)
 // Chi's URL parameter is normally populated by the production router.
 ackCtx:=chi.NewRouteContext()
 ackCtx.URLParams.Add("commandID",command.ID.String())
 ackReq=ackReq.WithContext(context.WithValue(ackReq.Context(),chi.RouteCtxKey,ackCtx))
 ackRec:=httptest.NewRecorder()
 (&AgentHandler{Repository:repository}).Acknowledge(ackRec,ackReq)
 if ackRec.Code!=http.StatusOK {t.Fatalf("agent acknowledgement: %d: %s",ackRec.Code,ackRec.Body.String())}
 var sessionState,commandState string
 if err:=db.QueryRow(ctx,`SELECT state FROM network_sessions WHERE id=$1`,httpSessionID).Scan(&sessionState);err!=nil {t.Fatal(err)}
 if err:=db.QueryRow(ctx,`SELECT state FROM network_disconnect_commands WHERE id=$1`,command.ID).Scan(&commandState);err!=nil {t.Fatal(err)}
 if sessionState!="disconnected" || commandState!="succeeded" {t.Fatalf("disconnect state mismatch: session=%s command=%s",sessionState,commandState)}
 reportTx,err:=db.Begin(ctx)
 if err!=nil {t.Fatal(err)}
 defer reportTx.Rollback(context.Background())
 first:=PersistentSession{
  AgentID:agent,AgentSessionID:"batch-first-"+suffix,
  UserID:userID,SubscriptionID:subscriptionID,ClientIP:ip,
  Mode:"sni",Protocol:"tcp",ConnectedAt:now,LastActivityAt:now,
  LastReportAt:time.Now().UTC(),UploadBytes:0,DownloadBytes:0,
 }
 if _,err:=upsertSession(ctx,reportTx,first);err!=nil {t.Fatalf("first batch write: %v",err)}
 second:=first
 second.AgentSessionID="batch-second-"+suffix
 second.SubscriptionID=uuid.New() // No matching entitlement.
 if _,err:=upsertSession(ctx,reportTx,second);err==nil {
  t.Fatal("expected second batch write to reject missing entitlement")
 }
 if err:=reportTx.Rollback(ctx);err!=nil {t.Fatal(err)}
 var rolledBack int
 if err:=db.QueryRow(ctx,`SELECT count(*) FROM network_sessions WHERE agent_id=$1 AND agent_session_id=$2`,agent,first.AgentSessionID).Scan(&rolledBack);err!=nil {t.Fatal(err)}
 if rolledBack!=0 {t.Fatal("first batch session persisted despite transaction rollback")}
}
