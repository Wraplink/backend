package networksessions

import (
 "context"
 "errors"
 "net/netip"
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
 AgentID string
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
// ResolveSNIOwner derives attribution from active backend entitlements rather
// than trusting user/subscription identifiers supplied by an agent. A shared
// client IP can correspond to multiple users; ambiguous ownership fails closed.
func (r *Repository) ResolveSNIOwner(ctx context.Context, clientIP string) (uuid.UUID, uuid.UUID, error) {
 return resolveSNIOwner(ctx,r.DB,clientIP)
}

// sessionQueryer is implemented by both pgxpool.Pool and pgx.Tx.
type sessionQueryer interface {
 Query(context.Context,string,...any)(pgx.Rows,error)
 QueryRow(context.Context,string,...any) pgx.Row
}

func resolveSNIOwner(ctx context.Context,q sessionQueryer, clientIP string) (uuid.UUID, uuid.UUID, error) {
 ip,parseErr:=netip.ParseAddr(clientIP)
 if parseErr!=nil || ip.Is4In6() || ip.Zone()!="" {return uuid.Nil,uuid.Nil,errors.New("invalid SNI client IP")}
 clientIP=ip.String()
 var owners []struct{ user, subscription uuid.UUID }
 rows, err := q.Query(ctx, `
 SELECT DISTINCT e.user_id,e.subscription_id
 FROM network_entitlements e
 JOIN user_subscriptions sub ON sub.id=e.subscription_id AND sub.user_id=e.user_id
 WHERE e.client_ip=$1::inet AND e.status='active'
 AND sub.status='active' AND sub.starts_at<=now() AND sub.expires_at>now()
 AND (sub.unlimited_bandwidth OR sub.used_bytes<sub.data_limit_bytes)
 LIMIT 2`, clientIP)
 if err != nil {return uuid.Nil,uuid.Nil,err}
 defer rows.Close()
 for rows.Next() {
  var owner struct{ user, subscription uuid.UUID }
  if err:=rows.Scan(&owner.user,&owner.subscription);err!=nil{return uuid.Nil,uuid.Nil,err}
  owners=append(owners,owner)
 }
 if err:=rows.Err();err!=nil{return uuid.Nil,uuid.Nil,err}
 if len(owners)!=1 {return uuid.Nil,uuid.Nil,errors.New("SNI session ownership is absent or ambiguous")}
 return owners[0].user,owners[0].subscription,nil
}

func (r *Repository) UpsertSession(ctx context.Context,s PersistentSession) (uuid.UUID,error) {
 return upsertSession(ctx,r.DB,s)
}

func upsertSession(ctx context.Context,q sessionQueryer,s PersistentSession) (uuid.UUID,error) {
 if s.AgentID=="" || s.UserID==uuid.Nil || s.SubscriptionID==uuid.Nil || s.AgentSessionID=="" || len(s.AgentSessionID)>256 || s.UploadBytes<0 || s.DownloadBytes<0 {return uuid.Nil,errors.New("invalid session")}
 if err:=validate(Session{AgentID:s.AgentID,SessionID:s.AgentSessionID,UserID:s.UserID.String(),SubscriptionID:s.SubscriptionID.String(),ClientIP:s.ClientIP,Mode:s.Mode,Protocol:s.Protocol,ConnectedAt:s.ConnectedAt,LastActivityAt:s.LastActivityAt},s.LastReportAt);err!=nil {return uuid.Nil,err}
 // The primary entitlement model is network_entitlements, shared by SNI
 // and route. Route additionally requires a matching active edge assignment.
 // The quota predicate respects the explicit unlimited_bandwidth flag.
 var authorized bool
 err:=q.QueryRow(ctx,`
 SELECT EXISTS (
  SELECT 1 FROM network_entitlements e
  JOIN user_subscriptions sub ON sub.id=e.subscription_id AND sub.user_id=e.user_id
  WHERE e.user_id=$2 AND e.subscription_id=$3
   AND e.client_ip=$4::inet AND e.status='active'
   AND sub.status='active' AND sub.starts_at<=now() AND sub.expires_at>now()
   AND (sub.unlimited_bandwidth OR sub.used_bytes<sub.data_limit_bytes)
   AND (
    $5='sni'
    OR ($5='route' AND EXISTS (
     SELECT 1 FROM network_access_assignments a
     WHERE a.edge_node_id=(SELECT id FROM edge_nodes WHERE code=$1 AND enabled) AND a.user_id=e.user_id
      AND a.subscription_id=e.subscription_id AND a.client_ip=e.client_ip
      AND a.state='active'
    ))
   )
 )
 `,s.AgentID,s.UserID,s.SubscriptionID,s.ClientIP,s.Mode).Scan(&authorized)
 if err!=nil {return uuid.Nil,err}
 if !authorized {return uuid.Nil,errors.New("session has no active matching entitlement")}
 var id uuid.UUID
 err=q.QueryRow(ctx,`
 INSERT INTO network_sessions(agent_id,agent_session_id,user_id,subscription_id,client_ip,mode,protocol,connected_at,last_activity_at,last_report_at,upload_bytes,download_bytes)
 VALUES($1,$2,$3,$4,$5::inet,$6,$7,$8,$9,$10,$11,$12)
 ON CONFLICT(agent_id,agent_session_id) DO UPDATE SET
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
 RETURNING id`,s.AgentID,s.AgentSessionID,s.UserID,s.SubscriptionID,s.ClientIP,s.Mode,s.Protocol,s.ConnectedAt,s.LastActivityAt,s.LastReportAt,s.UploadBytes,s.DownloadBytes).Scan(&id)
 if errors.Is(err,pgx.ErrNoRows) {return uuid.Nil,errors.New("session report rejected: stale, revoked or identity mismatch")}
 return id,err
}
// ListOnline uses the report timestamp, not website login state, to infer presence.
func (r *Repository) ListOnline(ctx context.Context,now time.Time,ttl time.Duration,limit int) ([]PersistentSession,error) {
 if ttl<=0 || limit<=0 || limit>500 {return nil,errors.New("invalid listing bounds")}
 rows,err:=r.DB.Query(ctx,`
 SELECT id,agent_id,agent_session_id,user_id,subscription_id,host(client_ip),mode,protocol,connected_at,last_activity_at,last_report_at,upload_bytes,download_bytes,state
 FROM network_sessions
 WHERE state='online' AND last_report_at >= $1
 ORDER BY last_report_at DESC,id DESC LIMIT $2`,now.Add(-ttl),limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=make([]PersistentSession,0)
 for rows.Next(){
  var s PersistentSession
  if err:=rows.Scan(&s.ID,&s.AgentID,&s.AgentSessionID,&s.UserID,&s.SubscriptionID,&s.ClientIP,&s.Mode,&s.Protocol,&s.ConnectedAt,&s.LastActivityAt,&s.LastReportAt,&s.UploadBytes,&s.DownloadBytes,&s.State);err!=nil{return nil,err}
  out=append(out,s)
 }
 return out,rows.Err()
}
