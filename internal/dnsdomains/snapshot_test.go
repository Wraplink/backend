package dnsdomains

import (
 "net/http"
 "net/http/httptest"
 "testing"
)

func TestManagedSnapshotUnauthorized(t *testing.T) {
 h := Handler{ControlKey:"secret"}
 r := httptest.NewRequest(http.MethodGet,"/managed-domains",nil)
 w := httptest.NewRecorder()
 h.Snapshot(w,r)
 if w.Code != http.StatusUnauthorized { t.Fatalf("status %d",w.Code) }
}
