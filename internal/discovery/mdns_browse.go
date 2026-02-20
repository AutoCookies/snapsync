package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// MDNSResolver discovers SnapSync peers over mDNS.
type MDNSResolver struct{}

// Browse discovers peers for timeout window.
func (r MDNSResolver) Browse(ctx context.Context, timeout time.Duration) ([]Peer, error) {
	maddr, err := net.ResolveUDPAddr("udp4", mdnsAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve mdns addr: %w", err)
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, maddr)
	if err != nil {
		return nil, fmt.Errorf("listen multicast: %w", err)
	}
	defer func() { _ = conn.Close() }()

	query := buildQuery(ServiceType + ".local")
	_, _ = conn.WriteToUDP(query, maddr)

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	seen := map[string]Peer{}
	buf := make([]byte, 65535)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _, readErr := conn.ReadFromUDP(buf)
		if readErr == nil && n > 0 {
			if peer, ok := parseAnnouncement(buf[:n]); ok {
				seen[peer.ID] = peer
			}
		}
		select {
		case <-ctxTimeout.Done():
			peers := make([]Peer, 0, len(seen))
			for _, p := range seen {
				peers = append(peers, p)
			}
			SortByFreshness(peers)
			return peers, nil
		default:
		}
	}
}

// ResolveByID resolves one peer by id.
func (r MDNSResolver) ResolveByID(ctx context.Context, id string) (Peer, error) {
	peers, err := r.Browse(ctx, 2*time.Second)
	if err != nil {
		return Peer{}, err
	}
	for _, peer := range peers {
		if peer.ID == id {
			return peer, nil
		}
	}
	return Peer{}, fmt.Errorf("peer %q not found", id)
}

type dnsQuestion struct {
	Name string
	Type uint16
}

type rr struct {
	Name  string
	Type  uint16
	RData []byte
}

func parseQuestions(packet []byte) ([]dnsQuestion, error) {
	if len(packet) < 12 {
		return nil, fmt.Errorf("dns packet too short")
	}
	qd := int(readU16(packet, 4))
	off := 12
	questions := make([]dnsQuestion, 0, qd)
	for i := 0; i < qd; i++ {
		name, next, err := readName(packet, off)
		if err != nil {
			return nil, err
		}
		off = next
		if off+4 > len(packet) {
			return nil, fmt.Errorf("truncated question")
		}
		qType := readU16(packet, off)
		off += 4
		questions = append(questions, dnsQuestion{Name: name, Type: qType})
	}
	return questions, nil
}

func parseAnnouncement(packet []byte) (Peer, bool) {
	rrs, err := parseRRs(packet)
	if err != nil {
		return Peer{}, false
	}
	var id, name string
	var port int
	addrs := []net.IP{}
	for _, record := range rrs {
		switch record.Type {
		case 16:
			fields := parseTXT(record.RData)
			if fields["ver"] != "1" || fields["id"] == "" {
				continue
			}
			id = fields["id"]
			name = fields["name"]
		case 33:
			if len(record.RData) < 7 {
				continue
			}
			port = int(readU16(record.RData, 4))
		case 1:
			if len(record.RData) == 4 {
				addrs = append(addrs, net.IPv4(record.RData[0], record.RData[1], record.RData[2], record.RData[3]))
			}
		}
	}
	if id == "" || port == 0 || len(addrs) == 0 {
		return Peer{}, false
	}
	if name == "" {
		name = "snapsync-peer"
	}
	return NewPeer(id, name, addrs, port, time.Now()), true
}

func parseRRs(packet []byte) ([]rr, error) {
	if len(packet) < 12 {
		return nil, fmt.Errorf("dns packet too short")
	}
	qd := int(readU16(packet, 4))
	an := int(readU16(packet, 6))
	ns := int(readU16(packet, 8))
	ar := int(readU16(packet, 10))
	off := 12
	for i := 0; i < qd; i++ {
		_, next, err := readName(packet, off)
		if err != nil {
			return nil, err
		}
		off = next + 4
	}
	total := an + ns + ar
	res := make([]rr, 0, total)
	for i := 0; i < total; i++ {
		name, next, err := readName(packet, off)
		if err != nil {
			return nil, err
		}
		off = next
		if off+10 > len(packet) {
			return nil, fmt.Errorf("truncated rr")
		}
		rType := readU16(packet, off)
		rdLen := int(readU16(packet, off+8))
		off += 10
		if off+rdLen > len(packet) {
			return nil, fmt.Errorf("truncated rdata")
		}
		rdata := append([]byte{}, packet[off:off+rdLen]...)
		off += rdLen
		res = append(res, rr{Name: name, Type: rType, RData: rdata})
	}
	return res, nil
}

func buildQuery(name string) []byte {
	msg := make([]byte, 12)
	setUint16(msg, 4, 1)
	msg = append(msg, encodeName(name)...)
	msg = append(msg, 0, 12, 0, 1)
	return msg
}

func parseTXT(rdata []byte) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(rdata); {
		l := int(rdata[i])
		i++
		if i+l > len(rdata) || l == 0 {
			break
		}
		entry := string(rdata[i : i+l])
		i += l
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func readName(packet []byte, off int) (string, int, error) {
	labels := []string{}
	orig := off
	jumped := false
	for {
		if off >= len(packet) {
			return "", 0, fmt.Errorf("name out of range")
		}
		l := int(packet[off])
		if l == 0 {
			off++
			break
		}
		if l&0xC0 == 0xC0 {
			if off+1 >= len(packet) {
				return "", 0, fmt.Errorf("bad pointer")
			}
			ptr := int(packet[off]&0x3F)<<8 | int(packet[off+1])
			if !jumped {
				orig = off + 2
				jumped = true
			}
			off = ptr
			continue
		}
		off++
		if off+l > len(packet) {
			return "", 0, fmt.Errorf("label out of range")
		}
		labels = append(labels, string(packet[off:off+l]))
		off += l
	}
	if jumped {
		return strings.Join(labels, ".") + ".", orig, nil
	}
	return strings.Join(labels, ".") + ".", off, nil
}

func readU16(b []byte, off int) uint16 { return uint16(b[off])<<8 | uint16(b[off+1]) }
