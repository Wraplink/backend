package networksessions

import (
 "testing"
 "time"
)
func sample(now time.Time) Session { return Session{AgentID:"agent-1",SessionID:"session-1",UserID:"user-1",SubscriptionID:"sub-1",ClientIP:"192.0.2.10",Mode:"sni",Protocol:"tcp",ConnectedAt:now.Add(-time.Minute),LastActivityAt:now.Add(-time.Second),UploadBytes:100,DownloadBytes:200} }
func TestPresenceAndExpiry(t *testing.T) {
 now:=time.Now().UTC()
 s,_:=NewStore(30*time.Second)
 if err:=s.Observe(sample(now),now);err!=nil {t.Fatal(err)}
 v,ok:=s.Get("agent-1","session-1",now.Add(20*time.Second));if !ok||v.Status!=Online {t.Fatalf("expected online: %+v",v)}
 v,_=s.Get("agent-1","session-1",now.Add(31*time.Second));if v.Status!=Unknown {t.Fatalf("expected unknown: %+v",v)}
}
func TestRejectReplayAndRevival(t *testing.T) {
 now:=time.Now().UTC()
 s,_:=NewStore(time.Minute)
 v:=sample(now)
 if err:=s.Observe(v,now);err!=nil {t.Fatal(err)}
 v.UploadBytes=99
 if err:=s.Observe(v,now.Add(time.Second));err==nil {t.Fatal("expected counter regression rejection")}
 if !s.MarkDisconnected("agent-1","session-1") {t.Fatal("missing session")}
 if err:=s.Observe(sample(now),now.Add(2*time.Second));err==nil {t.Fatal("disconnected session revived")}
}
func TestAgentScopedIdentity(t *testing.T) {
 now:=time.Now().UTC()
 s,_:=NewStore(time.Minute)
 a:=sample(now);b:=sample(now);b.AgentID="agent-2"
 if err:=s.Observe(a,now);err!=nil {t.Fatal(err)}
 if err:=s.Observe(b,now);err!=nil {t.Fatal(err)}
 s.MarkDisconnected(a.AgentID,a.SessionID)
 got,_:=s.Get(b.AgentID,b.SessionID,now)
 if got.Status!=Online {t.Fatal("other agent session affected")}
}
