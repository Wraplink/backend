package slackreport

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "log/slog"
 "net/http"
 "os"
 "time"

 "github.com/jackc/pgx/v5/pgxpool"
)

// Run dispatches database-backed events. Multiple replicas may run concurrently.
func Run(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger) {
 webhook := os.Getenv("WRAPLINK_SLACK_WEBHOOK_URL")
 if webhook == "" { logger.Info("Slack outbox delivery disabled: webhook not configured"); return }
 client := &http.Client{Timeout: 8 * time.Second}
 ticker := time.NewTicker(10 * time.Second)
 defer ticker.Stop()
 for {
  dispatch(ctx, db, client, webhook, logger)
  select { case <-ctx.Done(): return; case <-ticker.C: }
 }
}

func dispatch(ctx context.Context, db *pgxpool.Pool, client *http.Client, webhook string, logger *slog.Logger) {
 for i:=0;i<20;i++ {
  tx,err:=db.Begin(ctx); if err!=nil{return}
  var id int64
  var payload []byte
  err=tx.QueryRow(ctx,`SELECT id,payload FROM slack_outbox WHERE delivered_at IS NULL AND next_attempt_at<=now() ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id,&payload)
  if err!=nil { _=tx.Rollback(ctx);return }
  var event struct{Text string `json:"text"`}
  if json.Unmarshal(payload,&event)!=nil { _=tx.Rollback(ctx);logger.Error("invalid Slack outbox payload","id",id);return }
  body,_:=json.Marshal(map[string]string{"text":event.Text})
  req,err:=http.NewRequestWithContext(ctx,http.MethodPost,webhook,bytes.NewReader(body))
  if err==nil {req.Header.Set("Content-Type","application/json");var resp *http.Response;resp,err=client.Do(req);if err==nil {resp.Body.Close();if resp.StatusCode!=http.StatusOK {err=fmt.Errorf("Slack HTTP %d",resp.StatusCode)}}}
  if err==nil {
   _,err=tx.Exec(ctx,"UPDATE slack_outbox SET delivered_at=now() WHERE id=$1",id)
  } else {
   _,err=tx.Exec(ctx,`UPDATE slack_outbox SET attempts=attempts+1,next_attempt_at=now() + (LEAST(3600,POWER(2,LEAST(attempts,10))::integer) * interval '1 second') WHERE id=$1`,id)
   logger.Warn("Slack delivery deferred","event_id",id)
  }
  if err!=nil {_=tx.Rollback(ctx);return}
  if err=tx.Commit(ctx);err!=nil{return}
 }
}
