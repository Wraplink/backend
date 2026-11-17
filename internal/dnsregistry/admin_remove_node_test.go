package dnsregistry

import (
 "testing"
 "time"
)

func TestRemoveInactiveDNSNode(t *testing.T) {
 s:=NewStore()
 if _,ok:=s.Upsert("dns-live","local",[]string{"192.0.2.10"});!ok {t.Fatal("upsert failed")}
 found,removed:=s.RemoveInactive("dns-live",time.Minute)
 if !found||removed {t.Fatalf("live node must not be removed: found=%v removed=%v",found,removed)}
 if len(s.List(0))!=1 {t.Fatal("live node disappeared")}
 found,removed=s.RemoveInactive("missing",time.Minute)
 if found||removed {t.Fatal("missing node should not be removed")}
 // A negative threshold must never permit deletion.
 found,removed=s.RemoveInactive("dns-live",0)
 if !found||removed {t.Fatal("zero threshold should reject deletion")}
 s.mu.Lock()
 n:=s.nodes["dns-live"]
 n.LastSeenAt=time.Now().UTC().Add(-2*time.Minute)
 s.nodes["dns-live"]=n
 s.mu.Unlock()
 found,removed=s.RemoveInactive("dns-live",time.Minute)
 if !found||!removed {t.Fatal("stale node should be removed")}
 if len(s.List(0))!=0 {t.Fatal("stale node still present")}
}
