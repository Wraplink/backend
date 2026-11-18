package networksessions

import (
 "testing"
 "sync"
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

func TestValidationAndSessionIdentity(t *testing.T) {
 now:=time.Now().UTC()
 s,_:=NewStore(time.Minute)
 v:=sample(now)
 v.ClientIP="invalid"
 if err:=s.Observe(v,now);err==nil {t.Fatal("invalid IP accepted")}
 v=sample(now)
 if err:=s.Observe(v,now);err!=nil {t.Fatal(err)}
 v.ClientIP="192.0.2.11"
 if err:=s.Observe(v,now.Add(time.Second));err==nil {t.Fatal("session identity changed")}
 v=sample(now)
 if err:=s.Observe(v,now);err==nil {t.Fatal("duplicate report accepted")}
}
func TestListSnapshotAndStaleStatus(t *testing.T) {
 now:=time.Now().UTC()
 s,_:=NewStore(10*time.Second)
 a:=sample(now);b:=sample(now);b.AgentID="agent-2"
 if err:=s.Observe(a,now);err!=nil {t.Fatal(err)}
 if err:=s.Observe(b,now);err!=nil {t.Fatal(err)}
 if got:=s.List(now,Online);len(got)!=2 {t.Fatalf("want 2 online, got %d",len(got))}
 s.MarkDisconnected("agent-1","session-1")
 if got:=s.List(now.Add(11*time.Second),Online);len(got)!=0 {t.Fatalf("stale connection shown online: %d",len(got))}
 if got:=s.List(now.Add(11*time.Second),Unknown);len(got)!=1 || got[0].AgentID!="agent-2" {t.Fatalf("wrong unknown snapshot: %+v",got)}
 if got:=s.List(now.Add(11*time.Second),Disconnected);len(got)!=1 || got[0].AgentID!="agent-1" {t.Fatalf("wrong disconnected snapshot: %+v",got)}
}

func TestValidateSessionModesAndTimeBounds(t *testing.T) {
 now:=time.Date(2026,11,18,12,0,0,0,time.UTC)
 cases:=[]struct{name string;change func(*Session);wantError bool}{
  {"valid SNI",func(*Session){},false},
  {"valid route UDP",func(s *Session){s.Mode="route";s.Protocol="udp"},false},
  {"SNI UDP",func(s *Session){s.Protocol="udp"},true},
  {"invalid mode",func(s *Session){s.Mode="other"},true},
  {"invalid IP",func(s *Session){s.ClientIP="not-an-ip"},true},
  {"activity before connection",func(s *Session){s.LastActivityAt=s.ConnectedAt.Add(-time.Second)},true},
  {"activity in future",func(s *Session){s.LastActivityAt=now.Add(time.Second)},true},
  {"missing agent",func(s *Session){s.AgentID=""},true},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   session:=sample(now)
   tc.change(&session)
   err:=validate(session,now)
   if (err!=nil)!=tc.wantError {t.Fatalf("validate error=%v, wantError=%v",err,tc.wantError)}
  })
 }
}

func TestConcurrentSessionReportsNeverRegressCounters(t *testing.T) {
 now:=time.Now().UTC()
 store,err:=NewStore(time.Minute)
 if err!=nil {t.Fatal(err)}
 initial:=sample(now)
 if err:=store.Observe(initial,now);err!=nil {t.Fatal(err)}
 const workers=32
 var wg sync.WaitGroup
 for i:=1;i<=workers;i++ {
  wg.Add(1)
  go func(i int) {
   defer wg.Done()
   observation:=initial
   observation.UploadBytes+=uint64(i)
   observation.DownloadBytes+=uint64(i)
   observation.LastActivityAt=now.Add(time.Duration(i)*time.Millisecond)
   _=store.Observe(observation,now.Add(time.Duration(i)*time.Millisecond))
  }(i)
 }
 wg.Wait()
 got,ok:=store.Get(initial.AgentID,initial.SessionID,now.Add(time.Second))
 if !ok {t.Fatal("session missing")}
 if got.UploadBytes!=initial.UploadBytes+workers ||
  got.DownloadBytes!=initial.DownloadBytes+workers {
  t.Fatalf("counters regressed: upload=%d download=%d",got.UploadBytes,got.DownloadBytes)
 }
 if !got.LastActivityAt.Equal(now.Add(workers*time.Millisecond)) {
  t.Fatalf("activity regressed: %v",got.LastActivityAt)
 }
}
