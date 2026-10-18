package networkentitlement

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/google/uuid"
)

func TestEventsRequiresAuthentication(t *testing.T) {
    handler := &Handler{}
    request := httptest.NewRequest(http.MethodGet, "/api/v1/me/network/events", nil)
    response := httptest.NewRecorder()
    handler.Events(response, request)
    if response.Code != http.StatusUnauthorized {
        t.Fatalf("expected 401 for anonymous events request, got %d", response.Code)
    }
}

func TestConnectionEventJSONContract(t *testing.T) {
    id := uuid.New()
    timestamp := time.Date(2026, time.October, 18, 14, 0, 0, 0, time.UTC)
    encoded, err := json.Marshal(ConnectionEvent{
        ID: id, Type: "success", Title: "Connection activated", Time: timestamp,
    })
    if err != nil {
        t.Fatal(err)
    }
    var decoded map[string]any
    if err := json.Unmarshal(encoded, &decoded); err != nil {
        t.Fatal(err)
    }
    for field, expected := range map[string]string{
        "id": id.String(),
        "type": "success",
        "title": "Connection activated",
        "time": "2026-10-18T14:00:00Z",
    } {
        if decoded[field] != expected {
            t.Errorf("%s = %v, want %q", field, decoded[field], expected)
        }
    }
}
