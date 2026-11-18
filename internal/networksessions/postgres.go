package networksessions

import (
 "context"
 "errors"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists agent-confirmed sessions and administrator commands.
// Authentication, entitlement checks and agent delivery belong to the HTTP/service layer.
type Repository struct { DB *pgxpool.Pool }
func NewRepository(db *pgxpool.Pool) (*Repository,error) {
 if db==nil {return nil,errors.New("database pool is required")}
 return &Repository{DB:db},nil
}
type PersistentSession struct {
 ID uuid.UUID
 EdgeNodeID uuid.UUID
 AgentSessionID string
 UserID uuid.UUID
 SubscriptionID uuid.UUID
 ClientIP string
 Mode string
 Protocol string
 ConnectedAt time.Time
 LastActivityAt time.Time
 LastReportAt time.Time
 UploadBytes int64
 DownloadBytes int64
 State Status
}
func (r *Repository) UpsertSession(ctx context.Context,s PersistentSession) (uuid.UUID,error) {
 if s.EdgeNodeID==uuid.Nil || s.UserID==uuid.Nil || s.SubscriptionID==uuid.Nil || s.AgentSessionID=="" || s.UploadBytes<0 || s.DownloadBytes<0 {return uuid.Nil,errors.New("invalid session")}
 if err:=validate(Session{AgentID:s.EdgeNodeID.String(),SessionID:s.AgentSessionID,UserID:s.UserID.String(),SubscriptionID:s.SubscriptionID.String(),ClientIP:s.ClientIP,Mode:s.Mode,Protocol:s.Protocol,ConnectedAt:s.ConnectedAt,LastActivityAt:s.LastActivityAt},s.LastReportAt);err!=nil {return uuid.Nil,err}
 // Only a live entitlement assigned to this exact edge node and client
 // address may create or refresh a network session. This prevents an agent
 // from inventing online users or attaching traffic to another subscription.
 var authorized bool
 err:=r.DB.QueryRow(ctx,`
 SELECT EXISTS (
 SELECT 1 FROM network_access_assignments a
 JOIN user_subscriptions s ON s.id=a.subscription_id AND s.user_id=a.user_id
 WHERE a.edge_node_id=$1 AND a.user_id=$2 AND a.subscription_id=$3
 AND a.client_ip=$4::inet AND a.state='active'
 AND s.status='active' AND s.starts_at<=now() AND s.expires_at>now()
 )`,s.EdgeNodeID,s.UserID,s.SubscriptionID,s.ClientIP).Scan(&authorized)
 if err!=nil {return uuid.Nil,err}
 if !authorized {return uuid.Nil,errors.New("session has no active matching entitlement")}
 var id uuid.UUID
 err=r.DB.QueryRow(ctx,`
 INSERT INTO network_sessions(edge_node_id,agent_session_id,user_id,subscription_id,client_ip,mode,protocol,connected_at,last_activity_at,last_report_at,upload_bytes,download_bytes)
 VALUES($1,$2,$3,$4,$5::inet,$6,$7,$8,$9,$10,$11,$12)
 ON CONFLICT(edge_node_id,agent_session_id) DO UPDATE SET
 last_activity_at=EXCLUDED.last_activity_at,last_report_at=EXCLUDED.last_report_at,
 upload_bytes=EXCLUDED.upload_bytes,download_bytes=EXCLUDED.download_bytes
 WHERE network_sessions.state='online'
 AND network_sessions.user_id=EXCLUDED.user_id
 AND network_sessions.subscription_id=EXCLUDED.subscription_id
 AND network_sessions.client_ip=EXCLUDED.client_ip
 AND network_sessions.mode=EXCLUDED.mode
 AND network_sessions.protocol=EXCLUDED.protocol
 AND network_sessions.connected_at=EXCLUDED.connected_at
 AND EXCLUDED.last_report_at>network_sessions.last_report_at
 AND EXCLUDED.last_activity_at>=network_sessions.last_activity_at
 AND EXCLUDED.upload_bytes>=network_sessions.upload_bytes
 AND EXCLUDED.download_bytes>=network_sessions.download_bytes
 RETURNING id`,s.EdgeNodeID,s.AgentSessionID,s.UserID,s.SubscriptionID,s.ClientIP,s.Mode,s.Protocol,s.ConnectedAt,s.LastActivityAt,s.LastReportAt,s.UploadBytes,s.DownloadBytes).Scan(&id)
 if errors.Is(err,pgx.ErrNoRows) {return uuid.Nil,errors.New("session report rejected: stale, revoked or identity mismatch")}
 return id,err
}
// ListOnline uses the report timestamp, not website login state, to infer presence.
func (r *Repository) ListOnline(ctx context.Context,now time.Time,ttl time.Duration,limit int) ([]PersistentSession,error) {
 if ttl<=0 || limit<=0 || limit>500 {return nil,errors.New("invalid listing bounds")}
 rows,err:=r.DB.Query(ctx,`
 SELECT id,edge_node_id,agent_session_id,user_id,subscription_id,client_ip::text,mode,protocol,connected_at,last_activity_at,last_report_at,upload_bytes,download_bytes,state
 FROM network_sessions
 WHERE state='online' AND last_report_at >= $1
 ORDER BY last_report_at DESC,id DESC LIMIT $2`,now.Add(-ttl),limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=make([]PersistentSession,0)
 for rows.Next(){
  var s PersistentSession
  if err:=rows.Scan(&s.ID,&s.EdgeNodeID,&s.AgentSessionID,&s.UserID,&s.SubscriptionID,&s.ClientIP,&s.Mode,&s.Protocol,&s.ConnectedAt,&s.LastActivityAt,&s.LastReportAt,&s.UploadBytes,&s.DownloadBytes,&s.State);err!=nil{return nil,err}
  out=append(out,s)
 }
 return out,rows.Err()
}
