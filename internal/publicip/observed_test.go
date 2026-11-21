package publicip

import (
 "net/http"
 "testing"
)

func TestObservedIPv4IgnoresSpoofedHeaders(t *testing.T) {
 r := &http.Request{RemoteAddr:"8.8.8.8:40000", Header:http.Header{}}
 r.Header.Set("X-Forwarded-For","1.1.1.1")
 r.Header.Set("X-Real-IP","1.1.1.1")
 got, err := ObservedIPv4(r)
 if err != nil || got != "8.8.8.8" { t.Fatalf("got %q, %v",got,err) }
}
func TestObservedIPv4RejectsPrivatePeer(t *testing.T) {
 r := &http.Request{RemoteAddr:"10.0.0.1:3000"}
 if _, err := ObservedIPv4(r); err == nil { t.Fatal("expected rejection") }
}
