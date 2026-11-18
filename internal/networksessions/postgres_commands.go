package networksessions

import (
 "context"
 "errors"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

type PersistentCommand struct {
 ID uuid.UUID
 NetworkSessionID uuid.UUID
 AgentSessionID string `json:"agentSessionId"`
 EdgeNodeID uuid.UUID
 ActorUserID uuid.UUID
 Reason string
 IdempotencyKey string
 State CommandStatus
 CreatedAt time.Time
 UpdatedAt time.Time
 AcknowledgedAt *time.Time
 ErrorCode *string
}

// SubmitDisconnect creates an idempotent pending command for an existing
// network session. The caller must authenticate and authorize ActorUserID.
func (r *Repository) SubmitDisconnect(ctx context.Context,sessionID,actorID uuid.UUID,reason,key string) (PersistentCommand,error) {
 if sessionID==uuid.Nil || actorID==uuid.Nil || len(reason)==0 || len(reason)>1000 || len(key)==0 || len(key)>128 {
  return PersistentCommand{},errors.New("invalid disconnect request")
 }
 var c PersistentCommand
 // First resolve an idempotent retry, even if the session has since
 // disconnected. This avoids reporting a false "not online" on retries.
 err:=r.DB.QueryRow(ctx,`
 SELECT id,network_session_id,edge_node_id,actor_user_id,reason,idempotency_key,state,created_at,updated_at,acknowledged_at,error_code
 FROM network_disconnect_commands WHERE actor_user_id=$1 AND idempotency_key=$2
 `,actorID,key).Scan(&c.ID,&c.NetworkSessionID,&c.EdgeNodeID,&c.ActorUserID,&c.Reason,&c.IdempotencyKey,&c.State,&c.CreatedAt,&c.UpdatedAt,&c.AcknowledgedAt,&c.ErrorCode)
 if err==nil {
  if c.NetworkSessionID!=sessionID || c.Reason!=reason {return PersistentCommand{},errors.New("idempotency key already used for another request")}
  return c,nil
 }
 if !errors.Is(err,pgx.ErrNoRows) {return PersistentCommand{},err}
 // Concurrent submissions use the unique actor/key constraint as the
 // serialization point. RETURNING works for both insert and conflict.
 err=r.DB.QueryRow(ctx,`
 INSERT INTO network_disconnect_commands(network_session_id,edge_node_id,actor_user_id,reason,idempotency_key)
 SELECT id,edge_node_id,$2,$3,$4 FROM network_sessions
 WHERE id=$1 AND state='online'
 AND last_report_at >= now() - interval '2 minutes'
 ON CONFLICT(actor_user_id,idempotency_key)
 DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
 RETURNING id,network_session_id,edge_node_id,actor_user_id,reason,idempotency_key,state,created_at,updated_at,acknowledged_at,error_code
 `,sessionID,actorID,reason,key).Scan(&c.ID,&c.NetworkSessionID,&c.EdgeNodeID,&c.ActorUserID,&c.Reason,&c.IdempotencyKey,&c.State,&c.CreatedAt,&c.UpdatedAt,&c.AcknowledgedAt,&c.ErrorCode)
 if errors.Is(err,pgx.ErrNoRows) {return PersistentCommand{},errors.New("session not online or not found")}
 if err!=nil{return PersistentCommand{},err}
 if c.NetworkSessionID!=sessionID || c.ActorUserID!=actorID || c.Reason!=reason {return PersistentCommand{},errors.New("idempotency key already used for another request")}
 return c,nil
}

// PendingForAgent is called only after authenticating the edge agent.
func (r *Repository) PendingForAgent(ctx context.Context,agentID uuid.UUID,limit int) ([]PersistentCommand,error) {
 if agentID==uuid.Nil || limit<=0 || limit>100 {return nil,errors.New("invalid agent or limit")}
 rows,err:=r.DB.Query(ctx,`
 SELECT c.id,c.network_session_id,s.agent_session_id,c.edge_node_id,c.actor_user_id,c.reason,c.idempotency_key,c.state,c.created_at,c.updated_at,c.acknowledged_at,c.error_code
 FROM network_disconnect_commands c
 JOIN network_sessions s ON s.id=c.network_session_id AND s.edge_node_id=c.edge_node_id
 WHERE c.edge_node_id=$1 AND c.state='pending' AND s.state='online'
 AND s.last_report_at >= now() - interval '2 minutes'
 AND c.created_at >= now() - interval '2 minutes'
 ORDER BY c.created_at,c.id LIMIT $2`,agentID,limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 var out []PersistentCommand
 for rows.Next(){
  var c PersistentCommand
  if err:=rows.Scan(&c.ID,&c.NetworkSessionID,&c.AgentSessionID,&c.EdgeNodeID,&c.ActorUserID,&c.Reason,&c.IdempotencyKey,&c.State,&c.CreatedAt,&c.UpdatedAt,&c.AcknowledgedAt,&c.ErrorCode);err!=nil{return nil,err}
  out=append(out,c)
 }
 return out,rows.Err()
}

// Acknowledge is agent-scoped and atomic. Success updates session presence
// only after the agent reports that the exact session was terminated.
func (r *Repository) Acknowledge(ctx context.Context,agentID,commandID uuid.UUID,outcome CommandStatus,errorCode string) error {
 if agentID==uuid.Nil || commandID==uuid.Nil || (outcome!=CommandSucceeded && outcome!=CommandFailed) {return errors.New("invalid acknowledgement")}
 if len(errorCode)>128 {return errors.New("error code too long")}
 tx,err:=r.DB.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 var sessionID uuid.UUID
 var state CommandStatus
 err=tx.QueryRow(ctx,`SELECT network_session_id,state FROM network_disconnect_commands WHERE id=$1 AND edge_node_id=$2 FOR UPDATE`,commandID,agentID).Scan(&sessionID,&state)
 if err!=nil{return err}
 if state==outcome {return tx.Commit(ctx)}
 if state!=CommandPending {return errors.New("command already finalized")}
 _,err=tx.Exec(ctx,`UPDATE network_disconnect_commands SET state=$3,updated_at=now(),acknowledged_at=now(),error_code=NULLIF($4,'') WHERE id=$1 AND edge_node_id=$2`,commandID,agentID,outcome,errorCode)
 if err!=nil{return err}
 if outcome==CommandSucceeded {
  // Only a still-active, exact session can be marked disconnected.
  // A stale acknowledgement must not claim success after the session changed.
  tag,updateErr:=tx.Exec(ctx,`UPDATE network_sessions SET state='disconnected',disconnected_at=now() WHERE id=$1 AND edge_node_id=$2 AND state='online'`,sessionID,agentID)
  if updateErr!=nil{return updateErr}
  if tag.RowsAffected()!=1 {return errors.New("session no longer active; acknowledgement rejected")}
 }
 return tx.Commit(ctx)
}

// ExpirePending prevents indefinite pending commands after an agent outage.
func (r *Repository) ExpirePending(ctx context.Context,olderThan time.Time) (int64,error) {
 if olderThan.IsZero() || olderThan.After(time.Now()) {return 0,errors.New("invalid expiry cutoff")}
 tag,err:=r.DB.Exec(ctx,`UPDATE network_disconnect_commands SET state='expired',updated_at=now() WHERE state='pending' AND created_at<$1`,olderThan)
 if err!=nil{return 0,err}
 return tag.RowsAffected(),nil
}

// GetCommand returns a command for an authenticated administrator to inspect.
// Authorization of the caller is mandatory before invoking this method.
func (r *Repository) GetCommand(ctx context.Context,id uuid.UUID) (PersistentCommand,error) {
 if id==uuid.Nil {return PersistentCommand{},errors.New("invalid command ID")}
 var c PersistentCommand
 err:=r.DB.QueryRow(ctx,`
 SELECT id,network_session_id,edge_node_id,actor_user_id,reason,idempotency_key,state,created_at,updated_at,acknowledged_at,error_code
 FROM network_disconnect_commands WHERE id=$1`,id).
 Scan(&c.ID,&c.NetworkSessionID,&c.EdgeNodeID,&c.ActorUserID,&c.Reason,&c.IdempotencyKey,&c.State,&c.CreatedAt,&c.UpdatedAt,&c.AcknowledgedAt,&c.ErrorCode)
 return c,err
}
