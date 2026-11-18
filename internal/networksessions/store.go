// Package networksessions tracks agent-confirmed network activity.
// This volatile store is a building block, not production persistence.
package networksessions

import (
 "errors"
 "net/netip"
 "sort"
 "sync"
 "time"
)

type Status string
const (
 Online Status = "online"
 Unknown Status = "unknown"
 Disconnected Status = "disconnected"
)
type Session struct {
 AgentID string
 SessionID string
 UserID string
 SubscriptionID string
 ClientIP string
 Mode string
 Protocol string
 ConnectedAt time.Time
 LastActivityAt time.Time
 UploadBytes uint64
 DownloadBytes uint64
 Status Status
}
type Store struct {
 mu sync.RWMutex
 sessions map[string]Session
 lastReport map[string]time.Time
 ttl time.Duration
}
func NewStore(ttl time.Duration) (*Store,error) {
 if ttl <= 0 { return nil,errors.New("ttl must be positive") }
 return &Store{sessions:make(map[string]Session),lastReport:make(map[string]time.Time),ttl:ttl},nil
}
func key(agentID, sessionID string) string { return agentID+"\x00"+sessionID }
func validate(v Session, observedAt time.Time) error {
 if v.AgentID=="" || v.SessionID=="" || v.UserID=="" || v.SubscriptionID=="" || observedAt.IsZero() { return errors.New("missing session identity or observation time") }
 if _,err:=netip.ParseAddr(v.ClientIP);err!=nil { return errors.New("invalid client IP") }
 if v.Mode!="sni" && v.Mode!="route" { return errors.New("invalid connection mode") }
 if v.Protocol!="tcp" && v.Protocol!="udp" { return errors.New("invalid protocol") }
 if v.Mode=="sni" && v.Protocol!="tcp" { return errors.New("SNI requires TCP") }
 if v.ConnectedAt.IsZero() || v.LastActivityAt.IsZero() || v.LastActivityAt.After(observedAt) || v.ConnectedAt.After(observedAt) || v.LastActivityAt.Before(v.ConnectedAt) { return errors.New("invalid session timestamps") }
 return nil
}
// Observe requires a caller-authenticated agent identity and validated entitlement.
// Agent clocks must be bounded by the caller before invoking Observe.
func (s *Store) Observe(v Session, observedAt time.Time) error {
 if err:=validate(v,observedAt);err!=nil { return err }
 k:=key(v.AgentID,v.SessionID)
 s.mu.Lock()
 defer s.mu.Unlock()
 if old,ok:=s.sessions[k];ok {
  if old.Status==Disconnected { return errors.New("disconnected session cannot be revived") }
  if old.UserID!=v.UserID || old.SubscriptionID!=v.SubscriptionID || old.ClientIP!=v.ClientIP || old.Mode!=v.Mode || old.Protocol!=v.Protocol || !old.ConnectedAt.Equal(v.ConnectedAt) { return errors.New("session identity changed") }
  if !observedAt.After(s.lastReport[k]) { return errors.New("duplicate or out-of-order report") }
  if v.LastActivityAt.Before(old.LastActivityAt) || v.UploadBytes<old.UploadBytes || v.DownloadBytes<old.DownloadBytes { return errors.New("activity or counters decreased") }
 }
 v.Status=Online
 s.sessions[k]=v
 s.lastReport[k]=observedAt
 return nil
}
func (s *Store) effective(v Session,k string,now time.Time) Session {
 if v.Status==Online && now.Sub(s.lastReport[k])>s.ttl { v.Status=Unknown }
 return v
}
func (s *Store) Get(agentID,sessionID string, now time.Time) (Session,bool) {
 s.mu.RLock()
 defer s.mu.RUnlock()
 k:=key(agentID,sessionID)
 v,ok:=s.sessions[k]
 if !ok { return Session{},false }
 return s.effective(v,k,now),true
}
// List returns a deterministic snapshot; statusFilter may be empty to include all.
// Pagination and user visibility checks belong in the authenticated HTTP layer.
func (s *Store) List(now time.Time,statusFilter Status) []Session {
 s.mu.RLock()
 defer s.mu.RUnlock()
 out:=make([]Session,0,len(s.sessions))
 for k,v:=range s.sessions {
  v=s.effective(v,k,now)
  if statusFilter=="" || v.Status==statusFilter { out=append(out,v) }
 }
 sort.Slice(out,func(i,j int)bool {
  if out[i].AgentID!=out[j].AgentID { return out[i].AgentID<out[j].AgentID }
  return out[i].SessionID<out[j].SessionID
 })
 return out
}
// MarkDisconnected is ONLY for agent-confirmed termination, never command submission.
func (s *Store) MarkDisconnected(agentID,sessionID string) bool {
 s.mu.Lock()
 defer s.mu.Unlock()
 k:=key(agentID,sessionID)
 v,ok:=s.sessions[k]
 if !ok { return false }
 v.Status=Disconnected
 s.sessions[k]=v
 return true
}
