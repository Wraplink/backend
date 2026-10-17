package dnsregistry

import (
	"testing"
	"time"
)

func TestSelectAddressesPrefersDifferentNodes(t *testing.T) {
	store := NewStore()
	if _, ok := store.Upsert("dns-a", "eu", []string{"203.0.113.10", "203.0.113.11"}); !ok {
		t.Fatal("register dns-a")
	}
	time.Sleep(time.Millisecond)
	if _, ok := store.Upsert("dns-b", "eu", []string{"198.51.100.20"}); !ok {
		t.Fatal("register dns-b")
	}

	got := store.SelectAddresses(2, time.Minute)
	if len(got) != 2 {
		t.Fatalf("expected two addresses, got %v", got)
	}
	if got[0] != "198.51.100.20" {
		t.Fatalf("expected newest node first, got %v", got)
	}
	if got[1] != "203.0.113.10" {
		t.Fatalf("expected second node before secondary address, got %v", got)
	}
}

func TestSelectAddressesCanUseMultipleAddressesFromOneNode(t *testing.T) {
	store := NewStore()
	if _, ok := store.Upsert("dns-a", "eu", []string{"203.0.113.10", "203.0.113.11"}); !ok {
		t.Fatal("register dns-a")
	}

	got := store.SelectAddresses(2, time.Minute)
	if len(got) != 2 || got[0] != "203.0.113.10" || got[1] != "203.0.113.11" {
		t.Fatalf("unexpected addresses: %v", got)
	}
}

func TestUpsertRejectsInvalidAddresses(t *testing.T) {
	store := NewStore()
	if _, ok := store.Upsert("dns-a", "eu", []string{"not-an-ip"}); ok {
		t.Fatal("expected invalid address registration to fail")
	}
}
