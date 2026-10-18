package networkaccess

import (
    "strings"
    "testing"
    "time"

    "backend/internal/dnsregistry"
)

func TestRouteReadinessUsesHealthyRegisteredNode(t *testing.T) {
    nodes := dnsregistry.NewStore()
    if _, ok := nodes.Upsert("dns-eu-1", "eu-1", []string{"185.80.196.100"}); !ok {
        t.Fatal("expected valid node")
    }
    h := &Handler{service: &Service{
        dnsControls: nodes,
        dnsControlOfflineAfter: time.Minute,
    }}

    response := h.checkRouteReadiness([]string{"185.80.196.100"})
    if !response.Success || !strings.Contains(response.Value, "dns-eu-1") {
        t.Fatalf("expected assigned healthy route, got %+v", response)
    }
    if !strings.Contains(response.Value, "client route not measured") {
        t.Fatalf("result must not claim end-user traceroute: %+v", response)
    }
}

func TestRouteReadinessRejectsUnregisteredAddress(t *testing.T) {
    nodes := dnsregistry.NewStore()
    _, _ = nodes.Upsert("dns-eu-1", "eu-1", []string{"185.80.196.100"})
    h := &Handler{service: &Service{
        dnsControls: nodes,
        dnsControlOfflineAfter: time.Minute,
    }}
    response := h.checkRouteReadiness([]string{"185.80.196.101"})
    if response.Success {
        t.Fatalf("unexpected successful route: %+v", response)
    }
}

func TestRouteReadinessRequiresHealthyNode(t *testing.T) {
    nodes := dnsregistry.NewStore()
    _, _ = nodes.Upsert("dns-eu-1", "eu-1", []string{"185.80.196.100"})
    h := &Handler{service: &Service{
        dnsControls: nodes,
        dnsControlOfflineAfter: time.Nanosecond,
    }}
    time.Sleep(time.Millisecond)
    response := h.checkRouteReadiness([]string{"185.80.196.100"})
    if response.Success {
        t.Fatalf("offline node accepted: %+v", response)
    }
}
