package networksessions

import (
 "testing"
 "sync"
 "strconv"
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

func TestConcurrentCommandIdempotency(t *testing.T) {
 now:=time.Now().UTC()
 store:=NewCommandStore()
 const workers=32
 results:=make(chan DisconnectCommand,workers)
 failures:=make(chan error,workers)
 var wg sync.WaitGroup
 for i:=0;i<workers;i++ {
  wg.Add(1)
  go func(i int) {
   defer wg.Done()
   request:=command(now)
   request.ID= "command-"+stringID(i)
   result,err:=store.Submit(request)
   if err!=nil {failures<-err;return}
   results<-result
  }(i)
 }
 wg.Wait()
 close(results)
 close(failures)
 for err:=range failures {t.Fatalf("concurrent submit: %v",err)}
 var first string
 count:=0
 for result:=range results {
  if first=="" {first=result.ID}
  if result.ID!=first || result.Status!=CommandPending {
   t.Fatalf("inconsistent idempotent result: %+v",result)
  }
  count++
 }
 if count!=workers {t.Fatalf("got %d successful submissions, want %d",count,workers)}
}

func stringID(n int) string {return strconv.Itoa(n)}

func TestConcurrentCommandResolutionHasSingleFinalOutcome(t *testing.T) {
 now:=time.Now().UTC()
 store:=NewCommandStore()
 submitted,err:=store.Submit(command(now))
 if err!=nil {t.Fatal(err)}
 const workers=24
 var wg sync.WaitGroup
 successes:=make(chan CommandStatus,workers)
 for i:=0;i<workers;i++ {
  wg.Add(1)
  go func(i int) {
   defer wg.Done()
   outcome:=CommandSucceeded
   if i%2==1 {outcome=CommandFailed}
   resolved,err:=store.Resolve(submitted.ID,submitted.AgentID,outcome,now.Add(time.Second))
   if err==nil {successes<-resolved.Status}
  }(i)
 }
 wg.Wait()
 close(successes)
 final,ok:=store.Get(submitted.ID)
 if !ok || (final.Status!=CommandSucceeded && final.Status!=CommandFailed) {
  t.Fatalf("unexpected final command: %+v",final)
 }
 count:=0
 for status:=range successes {
  if status!=final.Status {t.Fatalf("conflicting successful acknowledgement: %s vs %s",status,final.Status)}
  count++
 }
 if count==0 {t.Fatal("no acknowledgement succeeded")}
}
