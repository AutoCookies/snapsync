package discovery

import (
	"context"
	"net"
	"os"
	"strings"
	"time"
)

const mdnsAddr = "224.0.0.251:5353"

// Advertiser manages mDNS service registration.
type Advertiser struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// AdvertiseConfig describes service metadata.
type AdvertiseConfig struct {
	InstanceName string
	PeerID       string
	DisplayName  string
	Port         int
}

// StartAdvertise starts mDNS advertisement.
func StartAdvertise(cfg AdvertiseConfig) (*Advertiser, error) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &Advertiser{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(a.done)
		runAdvertiser(ctx, cfg)
	}()
	return a, nil
}

// Stop unregisters discovery advertisement.
func (a *Advertiser) Stop() {
	if a == nil {
		return
	}
	a.cancel()
	<-a.done
}

func runAdvertiser(ctx context.Context, cfg AdvertiseConfig) {
	udpAddr, err := net.ResolveUDPAddr("udp4", mdnsAddr)
	if err != nil {
		return
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, udpAddr)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadBuffer(65535)

	host, _ := os.Hostname()
	if host == "" {
		host = "snapsync-host"
	}
	instance := sanitizeLabel(cfg.InstanceName)
	target := sanitizeLabel(host) + ".local"
	service := ServiceType + ".local"
	txt := []string{"ver=1", "id=" + cfg.PeerID, "name=" + cfg.DisplayName, "features=direct"}
	announce := buildAnnouncement(instance, service, target, cfg.Port, txt)
	queryName := service
	buf := make([]byte, 65535)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, src, readErr := conn.ReadFromUDP(buf)
		if readErr == nil && n > 0 {
			if packetHasQuestion(buf[:n], queryName, 12) {
				_, _ = conn.WriteToUDP(announce, src)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = conn.WriteToUDP(announce, udpAddr)
		default:
		}
	}
}

func sanitizeLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "snapsync"
	}
	v = strings.ReplaceAll(v, ".", "-")
	return v
}

func buildAnnouncement(instance, service, target string, port int, txt []string) []byte {
	instFQDN := instance + "." + service
	aName := target
	if !strings.HasSuffix(aName, ".") {
		aName += "."
	}
	serviceFQDN := ensureDot(service)
	instFQDN = ensureDot(instFQDN)

	msg := make([]byte, 12)
	setUint16(msg, 2, 0x8400)
	setUint16(msg, 6, 4)

	msg = append(msg, encodeName(serviceFQDN)...)
	msg = appendRRHeader(msg, 12, 1, 120)
	msg = append(msg, u16(uint16(len(encodeName(instFQDN))))...)
	msg = append(msg, encodeName(instFQDN)...)

	srvRData := make([]byte, 6)
	setUint16(srvRData, 0, 0)
	setUint16(srvRData, 2, 0)
	setUint16(srvRData, 4, uint16(port))
	srvRData = append(srvRData, encodeName(aName)...)
	msg = append(msg, encodeName(instFQDN)...)
	msg = appendRRHeader(msg, 33, 1, 120)
	msg = append(msg, u16(uint16(len(srvRData)))...)
	msg = append(msg, srvRData...)

	txtRData := []byte{}
	for _, t := range txt {
		if len(t) > 255 {
			continue
		}
		txtRData = append(txtRData, byte(len(t)))
		txtRData = append(txtRData, []byte(t)...)
	}
	msg = append(msg, encodeName(instFQDN)...)
	msg = appendRRHeader(msg, 16, 1, 120)
	msg = append(msg, u16(uint16(len(txtRData)))...)
	msg = append(msg, txtRData...)

	ip := firstIPv4()
	if ip == nil {
		ip = net.ParseIP("127.0.0.1")
	}
	msg = append(msg, encodeName(aName)...)
	msg = appendRRHeader(msg, 1, 1, 120)
	msg = append(msg, u16(uint16(4))...)
	msg = append(msg, ip.To4()...)
	return msg
}

func appendRRHeader(msg []byte, rrType uint16, class uint16, ttl uint32) []byte {
	msg = append(msg, u16(rrType)...)
	msg = append(msg, u16(class)...)
	msg = append(msg, byte(ttl>>24), byte(ttl>>16), byte(ttl>>8), byte(ttl))
	return msg
}

func ensureDot(name string) string {
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

func firstIPv4() net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if v4 := ipNet.IP.To4(); v4 != nil {
				return v4
			}
		}
	}
	return nil
}

func setUint16(b []byte, off int, v uint16) { b[off], b[off+1] = byte(v>>8), byte(v) }
func u16(v uint16) []byte                   { return []byte{byte(v >> 8), byte(v)} }

func encodeName(name string) []byte {
	name = strings.TrimSuffix(name, ".")
	labels := strings.Split(name, ".")
	out := make([]byte, 0, len(name)+2)
	for _, label := range labels {
		if label == "" {
			continue
		}
		out = append(out, byte(len(label)))
		out = append(out, []byte(label)...)
	}
	out = append(out, 0)
	return out
}

func packetHasQuestion(packet []byte, fqdn string, qtype uint16) bool {
	questions, err := parseQuestions(packet)
	if err != nil {
		return false
	}
	for _, q := range questions {
		if strings.EqualFold(ensureDot(q.Name), ensureDot(fqdn)) && q.Type == qtype {
			return true
		}
	}
	return false
}
