package dnsregistry

import (
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

type Node struct {
	ID         string    `json:"id"`
	Region     string    `json:"region,omitempty"`
	Addresses  []string  `json:"addresses"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type Store struct {
	mu    sync.RWMutex
	nodes map[string]Node
}

func NewStore() *Store {
	return &Store{nodes: make(map[string]Node)}
}

func (s *Store) Upsert(id, region string, addresses []string) (Node, bool) {
	id = strings.TrimSpace(id)
	region = strings.TrimSpace(region)
	addresses = normalizeAddresses(addresses)
	if id == "" || len(id) > 128 || len(addresses) == 0 {
		return Node{}, false
	}

	node := Node{
		ID:         id,
		Region:     region,
		Addresses:  addresses,
		LastSeenAt: time.Now().UTC(),
	}

	s.mu.Lock()
	s.nodes[id] = node
	s.mu.Unlock()
	return node, true
}

func (s *Store) SelectAddresses(limit int, offlineAfter time.Duration) []string {
	if limit <= 0 || offlineAfter <= 0 {
		return nil
	}

	cutoff := time.Now().UTC().Add(-offlineAfter)

	s.mu.RLock()
	nodes := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		if node.LastSeenAt.Before(cutoff) {
			continue
		}
		copyNode := node
		copyNode.Addresses = append([]string(nil), node.Addresses...)
		nodes = append(nodes, copyNode)
	}
	s.mu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].LastSeenAt.Equal(nodes[j].LastSeenAt) {
			return nodes[i].ID < nodes[j].ID
		}
		return nodes[i].LastSeenAt.After(nodes[j].LastSeenAt)
	})

	result := make([]string, 0, limit)
	seen := make(map[string]struct{})
	appendAddress := func(value string) {
		if len(result) >= limit {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	// Prefer separate dns-control nodes for primary/secondary redundancy.
	for _, node := range nodes {
		if len(node.Addresses) > 0 {
			appendAddress(node.Addresses[0])
		}
	}

	// A multi-homed node may provide additional addresses when fewer nodes exist.
	for _, node := range nodes {
		for _, address := range node.Addresses[1:] {
			appendAddress(address)
		}
	}

	return result
}

func (s *Store) List(offlineAfter time.Duration) []Node {
	cutoff := time.Now().UTC().Add(-offlineAfter)

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		if offlineAfter > 0 && node.LastSeenAt.Before(cutoff) {
			continue
		}
		copyNode := node
		copyNode.Addresses = append([]string(nil), node.Addresses...)
		result = append(result, copyNode)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func normalizeAddresses(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		ip := net.ParseIP(value)
		if ip == nil {
			continue
		}
		value = ip.String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
