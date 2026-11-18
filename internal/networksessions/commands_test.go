package networksessions

import (
 "testing"
 "time"
)
func command(now time.Time) DisconnectCommand {
 return DisconnectCommand{ID:"command-1",AgentID:"agent-1",SessionID:"session-1",ActorID:"admin-1",Reason:"admin_request",IdempotencyKey:"request-1",CreatedAt:now}
}
func TestCommandLifecycle(t *testing.T) {
 now:=time.Now().UTC()
 s:=NewCommandStore()
 c,err:=s.Submit(command(now));if err!=nil || c.Status!=CommandPending {t.Fatalf("submit: %+v %v",c,err)}
 if _,err:=s.Resolve(c.ID,"agent-2",CommandSucceeded,now.Add(time.Second));err==nil {t.Fatal("wrong agent acknowledged")}
 c,err=s.Resolve(c.ID,"agent-1",CommandSucceeded,now.Add(time.Second));if err!=nil || c.Status!=CommandSucceeded {t.Fatalf("resolve: %+v %v",c,err)}
 if _,err:=s.Resolve(c.ID,"agent-1",CommandFailed,now.Add(2*time.Second));err==nil {t.Fatal("finalized command changed")}
}
func TestCommandIdempotency(t *testing.T) {
 now:=time.Now().UTC()
 s:=NewCommandStore()
 first,err:=s.Submit(command(now));if err!=nil {t.Fatal(err)}
 retry:=command(now);retry.ID="command-2"
 got,err:=s.Submit(retry);if err!=nil || got.ID!=first.ID {t.Fatalf("retry: %+v %v",got,err)}
 retry.SessionID="another-session"
 if _,err:=s.Submit(retry);err==nil {t.Fatal("idempotency key reused with changed target")}
}
func TestCommandExpiry(t *testing.T) {
 now:=time.Now().UTC()
 s:=NewCommandStore()
 c,_:=s.Submit(command(now))
 c,err:=s.Expire(c.ID,now.Add(time.Minute));if err!=nil || c.Status!=CommandExpired {t.Fatalf("expiry: %+v %v",c,err)}
 if _,err:=s.Resolve(c.ID,"agent-1",CommandSucceeded,now.Add(2*time.Minute));err==nil {t.Fatal("expired command acknowledged")}
}
