package dnsregistry

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

func TestAgentEndpointsRequireControlKey(t *testing.T) {
 h := NewHandler(NewStore(), "secret", 30*time.Second)
 tests := []struct {
  name string
  handler func(http.ResponseWriter,*http.Request)
  method string
  path string
 }{
  {"register",h.RegisterAgent,http.MethodPost,"/api/v1/internal/network/agents/register"},
  {"heartbeat",h.HeartbeatAgent,http.MethodPost,"/api/v1/internal/network/agents/heartbeat"},
  {"snapshot",h.AgentSnapshot,http.MethodGet,"/api/v1/internal/network/agents/snapshot"},
 }
 for _, tc := range tests {
  t.Run(tc.name,func(t *testing.T) {
   req:=httptest.NewRequest(tc.method,tc.path,strings.NewReader("{}"))
   rec:=httptest.NewRecorder()
   tc.handler(rec,req)
   if rec.Code!=http.StatusUnauthorized {t.Fatalf("expected unauthorized, got %d",rec.Code)}
  })
 }
}
