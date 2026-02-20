// Package discovery provides LAN peer discovery via mDNS.
package discovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"snapsync/internal/store"
)

const (
	// ServiceType is DNS-SD service for SnapSync.
	ServiceType = "_snapsync._tcp"
	// ServiceDomain is default local domain.
	ServiceDomain = "local."
)

// Peer describes one discovered SnapSync receiver.
type Peer struct {
	ID        string
	Name      string
	Addresses []string
	Port      int
	LastSeen  time.Time
}

// Resolver resolves discovery peers.
type Resolver interface {
	Browse(ctx context.Context, timeout time.Duration) ([]Peer, error)
	ResolveByID(ctx context.Context, id string) (Peer, error)
}

// LocalPeerID returns a stable local peer id.
func LocalPeerID() (string, error) {
	return store.LoadOrCreatePeerID(func() (string, error) {
		host, _ := os.Hostname()
		mac := primaryMAC()
		if host != "" && mac != "" {
			return makePeerID(host + "|" + mac), nil
		}
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate random bytes: %w", err)
		}
		return makePeerID(hex.EncodeToString(buf)), nil
	})
}

// NewPeer builds Peer from service metadata.
func NewPeer(id, name string, addresses []net.IP, port int, seen time.Time) Peer {
	parts := make([]string, 0, len(addresses))
	for _, ip := range addresses {
		parts = append(parts, ip.String())
	}
	return Peer{ID: id, Name: name, Addresses: parts, Port: port, LastSeen: seen}
}

// PreferredAddress returns best-effort address for connecting.
func (p Peer) PreferredAddress() string {
	for _, addr := range p.Addresses {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if isPrivateIPv4(ip) {
			return addr
		}
	}
	for _, addr := range p.Addresses {
		ip := net.ParseIP(addr)
		if ip != nil && (ip.IsLinkLocalUnicast() || ip.IsPrivate()) {
			return addr
		}
	}
	if len(p.Addresses) > 0 {
		return p.Addresses[0]
	}
	return ""
}

// SortByFreshness sorts peers by last seen descending.
func SortByFreshness(peers []Peer) {
	sort.Slice(peers, func(i, j int) bool { return peers[i].LastSeen.After(peers[j].LastSeen) })
}

func makePeerID(seed string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(seed)))
	return hex.EncodeToString(sum[:])[:12]
}

func primaryMAC() string {
	ifs, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifs {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) > 0 {
			return iface.HardwareAddr.String()
		}
	}
	return ""
}

func isPrivateIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 10 || (v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31) || (v4[0] == 192 && v4[1] == 168)
}
