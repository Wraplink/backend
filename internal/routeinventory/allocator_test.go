package routeinventory

import "testing"

func TestAllocateWaitingAndReassignment(t *testing.T) {
 routes:=[]string{"a.example","b.example","c.example","d.example"}
 inv:=map[string][]string{"agent-1":{"192.0.2.1","192.0.2.2","192.0.2.3"}}
 first,waiting:=Allocate(routes,inv,nil)
 if len(first)!=3||len(waiting)!=1||waiting[0]!="d.example" {t.Fatalf("first: %+v waiting=%v",first,waiting)}
 inv["agent-2"]=[]string{"198.51.100.1","198.51.100.2"}
 second,waiting:=Allocate(routes,inv,first)
 if len(second)!=4||len(waiting)!=0||second[3].AgentID!="agent-2" {t.Fatalf("second: %+v waiting=%v",second,waiting)}
 inv=map[string][]string{"agent-2":{"198.51.100.1","198.51.100.2"}}
 third,waiting:=Allocate(routes,inv,second)
 if len(third)!=2||len(waiting)!=2 {t.Fatalf("failover: %+v waiting=%v",third,waiting)}
 for _,a:=range third {if a.AgentID!="agent-2" {t.Fatalf("offline agent retained: %+v",a)}}
}
func TestAllocateNoDuplicateAddress(t *testing.T) {
 got,waiting:=Allocate([]string{"a.example","b.example"},map[string][]string{"a":{"192.0.2.1"},"b":{"192.0.2.1"}},nil)
 if len(got)!=1||len(waiting)!=1 {t.Fatalf("duplicate IP assigned: %+v waiting=%v",got,waiting)}
}
