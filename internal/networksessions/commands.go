package networksessions

import (
 "errors"
 "sync"
 "time"
)

type CommandStatus string
const (
 CommandPending CommandStatus = "pending"
 CommandSucceeded CommandStatus = "succeeded"
 CommandFailed CommandStatus = "failed"
 CommandExpired CommandStatus = "expired"
)
type DisconnectCommand struct {
 ID string
 AgentID string
 SessionID string
 ActorID string
 Reason string
 IdempotencyKey string
 Status CommandStatus
 CreatedAt time.Time
 UpdatedAt time.Time
}
type CommandStore struct {
 mu sync.Mutex
 commands map[string]DisconnectCommand
 byRequest map[string]string
}
func NewCommandStore() *CommandStore {
 return &CommandStore{commands:make(map[string]DisconnectCommand),byRequest:make(map[string]string)}
}
// Submit must be called after verifying administrator privileges and target ownership.
// The caller supplies a server-generated unique command ID.
func (s *CommandStore) Submit(c DisconnectCommand) (DisconnectCommand,error) {
 if c.ID=="" || c.AgentID=="" || c.SessionID=="" || c.ActorID=="" || c.IdempotencyKey=="" || c.CreatedAt.IsZero() { return DisconnectCommand{},errors.New("missing command fields") }
 k:=c.ActorID+"\x00"+c.IdempotencyKey
 s.mu.Lock()
 defer s.mu.Unlock()
 if id,ok:=s.byRequest[k];ok {
  previous:=s.commands[id]
  if previous.AgentID!=c.AgentID || previous.SessionID!=c.SessionID || previous.Reason!=c.Reason { return DisconnectCommand{},errors.New("idempotency key reused for another request") }
  return previous,nil
 }
 if _,ok:=s.commands[c.ID];ok { return DisconnectCommand{},errors.New("duplicate command ID") }
 c.Status=CommandPending
 c.UpdatedAt=c.CreatedAt
 s.commands[c.ID]=c
 s.byRequest[k]=c.ID
 return c,nil
}
func (s *CommandStore) Get(id string) (DisconnectCommand,bool) {
 s.mu.Lock()
 defer s.mu.Unlock()
 c,ok:=s.commands[id]
 return c,ok
}
// Resolve accepts only a verified acknowledgement from the command's assigned agent.
// Caller must authenticate the agent and confirm the target session was terminated.
func (s *CommandStore) Resolve(id,agentID string,status CommandStatus,now time.Time) (DisconnectCommand,error) {
 if status!=CommandSucceeded && status!=CommandFailed { return DisconnectCommand{},errors.New("invalid acknowledgement status") }
 s.mu.Lock()
 defer s.mu.Unlock()
 c,ok:=s.commands[id]
 if !ok { return DisconnectCommand{},errors.New("unknown command") }
 if c.AgentID!=agentID { return DisconnectCommand{},errors.New("agent mismatch") }
 if c.Status!=CommandPending {
  if c.Status==status { return c,nil }
  return DisconnectCommand{},errors.New("command already finalized")
 }
 if now.Before(c.CreatedAt) { return DisconnectCommand{},errors.New("invalid acknowledgement time") }
 c.Status=status
 c.UpdatedAt=now
 s.commands[id]=c
 return c,nil
}
func (s *CommandStore) Expire(id string,now time.Time) (DisconnectCommand,error) {
 s.mu.Lock()
 defer s.mu.Unlock()
 c,ok:=s.commands[id]
 if !ok { return DisconnectCommand{},errors.New("unknown command") }
 if c.Status!=CommandPending { return c,nil }
 if now.Before(c.CreatedAt) { return DisconnectCommand{},errors.New("invalid expiry time") }
 c.Status=CommandExpired
 c.UpdatedAt=now
 s.commands[id]=c
 return c,nil
}
