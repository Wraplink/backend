// Package networksessions contains the agent-observed network presence model.
// It does not infer network presence from website authentication.
package networksessions

import (
 "errors"
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
// Observe records an agent-confirmed session. Callers must authenticate agentID,
// validate user/subscription ownership, and reject future or stale timestamps.
func (s *Store) Observe(v Session, observedAt time.Time) error {
 if v.AgentID=="" || v.SessionID=="" || v.UserID=="" || v.SubscriptionID=="" || observedAt.IsZero() { return errors.New("missing session identity or observation time") }
 if v.LastActivityAt.After(observedAt) || v.ConnectedAt.After(observedAt) || v.LastActivityAt.Before(v.ConnectedAt) { return errors.New("invalid session timestamps") }
 k:=key(v.AgentID,v.SessionID)
 s.mu.Lock()
 defer s.mu.Unlock()
 if old,ok:=s.sessions[k];ok {
  if old.Status==Disconnected { return errors.New("disconnected session cannot be revived") }
  if old.UserID!=v.UserID || old.SubscriptionID!=v.SubscriptionID { return errors.New("session identity changed") }
  if observedAt.Before(s.lastReport[k]) { return errors.New("out-of-order report") }
  if v.UploadBytes<old.UploadBytes || v.DownloadBytes<old.DownloadBytes { return errors.New("counters decreased") }
 }
 v.Status=Online
 s.sessions[k]=v
 s.lastReport[k]=observedAt
 return nil
}
func (s *Store) Get(agentID,sessionID string, now time.Time) (Session,bool) {
 s.mu.RLock()
 defer s.mu.RUnlock()
 k:=key(agentID,sessionID)
 v,ok:=s.sessions[k]
 if !ok { return Session{},false }
 if v.Status==Online && now.Sub(s.lastReport[k])>s.ttl { v.Status=Unknown }
 return v,true
}
// MarkDisconnected must be called only after agent-confirmed termination.
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
