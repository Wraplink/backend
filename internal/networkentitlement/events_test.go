package networkentitlement

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/internal/requestcontext"
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
		"id":    id.String(),
		"type":  "success",
		"title": "Connection activated",
		"time":  "2026-10-18T14:00:00Z",
	} {
		if decoded[field] != expected {
			t.Errorf("%s = %v, want %q", field, decoded[field], expected)
		}
	}
}

func TestEventCursorRoundTrip(t *testing.T) {
	event := ConnectionEvent{
		ID:   uuid.New(),
		Time: time.Date(2026, time.October, 18, 14, 0, 0, 123456789, time.FixedZone("IR", 3*3600+30*60)),
	}
	cursor, err := parseEventCursor(encodeEventCursor(event))
	if err != nil {
		t.Fatal(err)
	}
	if cursor.ID != event.ID || !cursor.Time.Equal(event.Time) {
		t.Fatalf("cursor = %+v, want ID %s at %s", cursor, event.ID, event.Time)
	}
}

func TestEventCursorRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"not-base64!", "Zm9v", "MjAyNi0xMC0xOFQxNDowMDowMFp8bm90LWEtdXVpZA", strings.Repeat("a", 201)} {
		if _, err := parseEventCursor(value); err == nil {
			t.Errorf("expected invalid cursor %q to fail", value)
		}
	}
}

func TestEventsRejectsInvalidPaginationBeforeDatabase(t *testing.T) {
	handler := &Handler{}
	for _, query := range []string{"?limit=zero", "?limit=-2", "?before=broken!"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/me/network/events"+query, nil)
		request = request.WithContext(requestcontext.WithUserID(request.Context(), uuid.New()))
		response := httptest.NewRecorder()
		handler.Events(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s returned %d, want 400", query, response.Code)
		}
	}
}
