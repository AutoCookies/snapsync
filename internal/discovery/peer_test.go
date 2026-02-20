package discovery

import (
	"net"
	"regexp"
	"testing"
)

func TestMakePeerIDDeterministicAndFormat(t *testing.T) {
	id1 := makePeerID("host|aa:bb:cc:dd:ee:ff")
	id2 := makePeerID("host|aa:bb:cc:dd:ee:ff")
	if id1 != id2 {
		t.Fatalf("expected deterministic id, got %q and %q", id1, id2)
	}
	if ok, _ := regexp.MatchString(`^[a-f0-9]{12}$`, id1); !ok {
		t.Fatalf("id format invalid: %q", id1)
	}
}

func TestParseTXTAndAnnouncement(t *testing.T) {
	txt := parseTXT([]byte{5, 'v', 'e', 'r', '=', '1', 15, 'i', 'd', '=', 'a', '1', 'b', '2', 'c', '3', 'd', '4', 'e', '5', 'f', '6', 9, 'n', 'a', 'm', 'e', '=', 'L', 'a', 'p', 't', 'o', 'p'})
	if txt["ver"] != "1" || txt["id"] != "a1b2c3d4e5f6" {
		t.Fatalf("unexpected txt parse: %#v", txt)
	}

	pkt := buildAnnouncement("Laptop", ServiceType+".local", "host.local", 45999, []string{"ver=1", "id=a1b2c3d4e5f6", "name=Laptop", "features=direct"})
	peer, ok := parseAnnouncement(pkt)
	if !ok {
		t.Fatal("expected valid announcement parse")
	}
	if peer.ID != "a1b2c3d4e5f6" || peer.Port != 45999 || peer.Name != "Laptop" {
		t.Fatalf("unexpected peer: %#v", peer)
	}
	if len(peer.Addresses) == 0 || net.ParseIP(peer.Addresses[0]) == nil {
		t.Fatalf("expected parseable address, got %#v", peer.Addresses)
	}
}
