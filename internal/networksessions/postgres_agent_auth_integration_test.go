package networksessions

import (
 "context"
 "crypto/sha256"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Checks real HTTP authentication and report parsing against migrated Postgres.
// The agent is deliberately isolated from production identities.
func TestAgentReportAuthenticationPostgres(t *testing.T) {
 dsn:=os.Getenv("TEST_DATABASE_URL")
 if dsn=="" {t.Skip("TEST_DATABASE_URL not configured")}
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second)
 defer cancel()
 db,err:=pgxpool.New(ctx,dsn)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 agentID:="auth-test-"+uuid.NewString()
 token:="network-sessions-integration-"+uuid.NewString()
 digest:=sha256.Sum256([]byte(token))
 if _,err:=db.Exec(ctx,`INSERT INTO network_agent_health(id,status,last_seen_at) VALUES($1,'online',now())`,agentID);err!=nil {t.Fatal(err)}
 defer func(){
  cleanupCtx,done:=context.WithTimeout(context.Background(),5*time.Second)
  defer done()
  if _,err:=db.Exec(cleanupCtx,`DELETE FROM network_agent_health WHERE id=$1`,agentID);err!=nil {t.Errorf("cleanup agent: %v",err)}
 }()
 if _,err:=db.Exec(ctx,`INSERT INTO network_session_agent_credentials(agent_id,token_sha256,enabled) VALUES($1,$2,true)`,agentID,digest[:]);err!=nil {t.Fatal(err)}
 h:=&AgentHandler{Repository:&Repository{DB:db}}
 call:=func(agent,secret,payload string)int{
  req:=httptest.NewRequest(http.MethodPost,"/api/v1/internal/network/sessions/report",strings.NewReader(payload))
  req.Header.Set("X-Network-Agent-ID",agent)
  req.Header.Set("Authorization","Bearer "+secret)
  rec:=httptest.NewRecorder()
  h.Report(rec,req)
  return rec.Code
 }
 // Authentication is checked before parsing and before touching sessions.
 if got:=call(agentID,"incorrect","{}");got!=http.StatusUnauthorized {t.Fatalf("incorrect token: %d",got)}
 if got:=call("other-agent",token,"{}");got!=http.StatusUnauthorized {t.Fatalf("incorrect agent: %d",got)}
 if got:=call(agentID,token,`{"sessions":[]}`);got!=http.StatusBadRequest {t.Fatalf("empty batch: %d",got)}
 if got:=call(agentID,token,`{"sessions":[],"unexpected":true}`);got!=http.StatusBadRequest {t.Fatalf("unknown field: %d",got)}
 if got:=call(agentID,token,`{"sessions":[{"sessionId":"x","clientIp":"not-an-ip","mode":"sni","protocol":"tcp"}]}`);got!=http.StatusBadRequest {t.Fatalf("invalid report: %d",got)}
 // An authenticated SNI report with no active entitlement must fail
 // closed: the agent cannot choose an owner for an arbitrary client IP.
 now:=time.Now().UTC().Format(time.RFC3339Nano)
 report:=`{"sessions":[{"sessionId":"unauthorized-owner","clientIp":"192.0.2.44","mode":"sni","protocol":"tcp","connectedAt":"`+now+`","lastActivityAt":"`+now+`","uploadBytes":0,"downloadBytes":0}]}`
 if got:=call(agentID,token,report);got!=http.StatusConflict {
  t.Fatalf("SNI report without entitlement must be rejected: %d",got)
 }
 var count int
 if err:=db.QueryRow(ctx,`SELECT count(*) FROM network_sessions WHERE agent_id=$1`,agentID).Scan(&count);err!=nil {t.Fatal(err)}
 if count!=0 {t.Fatalf("invalid reports created %d sessions",count)}
}
