package routeinventory

import ("testing";"fmt";"reflect")

func TestAllocateWaitingAndReassignment(t *testing.T) {
	routes := []string{"a.example", "b.example", "c.example", "d.example"}
	inv := map[string][]string{"agent-1": {"192.0.2.1", "192.0.2.2", "192.0.2.3"}}
	first, waiting := Allocate(routes, inv, nil)
	if len(first) != 3 || len(waiting) != 1 || waiting[0] != "d.example" {
		t.Fatalf("first: %+v waiting=%v", first, waiting)
	}
	inv["agent-2"] = []string{"198.51.100.1", "198.51.100.2"}
	second, waiting := Allocate(routes, inv, first)
	if len(second) != 4 || len(waiting) != 0 || second[3].AgentID != "agent-2" {
		t.Fatalf("second: %+v waiting=%v", second, waiting)
	}
	inv = map[string][]string{"agent-2": {"198.51.100.1", "198.51.100.2"}}
	third, waiting := Allocate(routes, inv, second)
	if len(third) != 2 || len(waiting) != 2 {
		t.Fatalf("failover: %+v waiting=%v", third, waiting)
	}
	for _, a := range third {
		if a.AgentID != "agent-2" {
			t.Fatalf("offline agent retained: %+v", a)
		}
	}
}
func TestAllocateNoDuplicateAddress(t *testing.T) {
	got, waiting := Allocate([]string{"a.example", "b.example"}, map[string][]string{"a": {"192.0.2.1"}, "b": {"192.0.2.1"}}, nil)
	if len(got) != 1 || len(waiting) != 1 {
		t.Fatalf("duplicate IP assigned: %+v waiting=%v", got, waiting)
	}
}

func TestAllocateSkipsUnresolvedRoutesAndReleasesPreviousIP(t *testing.T) {
	inventory := map[string][]string{"edge-a": {"192.0.2.10", "192.0.2.11"}}
	previous := []Assignment{
		{Domain: "unresolved.example", AgentID: "edge-a", PublicIP: "192.0.2.10"},
	}
	// The reconciler excludes domains without a resolved destination before
	// calling Allocate. Their previous IP must become available to other routes.
	assigned, waiting := Allocate([]string{"ready.example"}, inventory, previous)
	if len(waiting) != 0 || len(assigned) != 1 {
		t.Fatalf("expected only the resolvable route: assigned=%+v waiting=%v", assigned, waiting)
	}
	if assigned[0].Domain != "ready.example" || assigned[0].PublicIP != "192.0.2.10" {
		t.Fatalf("released IP should be reusable: %+v", assigned)
	}
}

func TestAllocateLargeFleetDeterministicFailover(t *testing.T) {
 const agents=250
 const routesCount=1000
 inventory:=make(map[string][]string,agents)
 routes:=make([]string,0,routesCount)
 for i:=0;i<agents;i++ {
  id:=fmt.Sprintf("edge-%04d",i)
  inventory[id]=[]string{
   fmt.Sprintf("10.%d.%d.1",i/250,(i%250)+1),
   fmt.Sprintf("10.%d.%d.2",i/250,(i%250)+1),
   fmt.Sprintf("10.%d.%d.3",i/250,(i%250)+1),
   fmt.Sprintf("10.%d.%d.4",i/250,(i%250)+1),
  }
 }
 for i:=0;i<routesCount;i++ {routes=append(routes,fmt.Sprintf("route-%04d.example",i))}
 first,waiting:=Allocate(routes,inventory,nil)
 if len(first)!=routesCount||len(waiting)!=0 {t.Fatalf("initial fleet: %d assigned, %d waiting",len(first),len(waiting))}
 seen:=map[string]bool{}
 for _,a:=range first {
  if seen[a.PublicIP] {t.Fatalf("duplicate IP %s",a.PublicIP)}
  seen[a.PublicIP]=true
 }
 delete(inventory,"edge-0000")
 second,waiting:=Allocate(routes,inventory,first)
 if len(second)!=routesCount-4 || len(waiting)!=4 {t.Fatalf("after failure: %d assigned, %d waiting",len(second),len(waiting))}
 for _,a:=range second {if a.AgentID=="edge-0000" {t.Fatal("failed agent retained")}}
 again,waitingAgain:=Allocate(routes,inventory,first)
 if !reflect.DeepEqual(second,again)||!reflect.DeepEqual(waiting,waitingAgain) {t.Fatal("allocation is not deterministic")}
}
