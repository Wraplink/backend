package networkaccess

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/internal/dnsregistry"
	"github.com/go-chi/chi/v5"
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

func TestDiagnosticRejectsUnknownType(t *testing.T) {
	h := &Handler{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/me/network/diagnostics/unknown", nil)
	// Route parameters are populated by chi; check the route below with its router.
	router := chi.NewRouter()
	router.Post("/api/v1/me/network/diagnostics/{type}", h.Diagnostic)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestDiagnosticRequiresAuthentication(t *testing.T) {
	h := &Handler{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/me/network/diagnostics/ping", nil)
	router := chi.NewRouter()
	router.Post("/api/v1/me/network/diagnostics/{type}", h.Diagnostic)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}
