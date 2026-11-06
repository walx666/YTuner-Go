package ytuner

// DNS proxy server (src/dnsserver.pas).
//
// Queries for the intercepted domains (InterceptDNs, e.g. *.vtuner.com) are
// answered with the IP address of the DNS service; all other queries are
// forwarded to the upstream servers (DNSServers) and the answer is relayed.

import (
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DNSService       = "DNS Service"
	DefDNSServerPort = 53
	DefInterceptDNs  = "*.vtuner.com"
	DefDNSServers    = "8.8.8.8,9.9.9.9"
)

var (
	DNSServerIPAddress string
	DNSServerPort      = DefDNSServerPort
	DNSServerEnabled   = true
	DNSServers         = DefDNSServers
	InterceptDNs       = DefInterceptDNs

	dnsMu   sync.Mutex
	dnsConn *net.UDPConn
)

// dnsAnswerTemplate mirrors DNSAnswerBytes (pointer to the question name,
// type A, class IN, TTL 3600, 4 bytes IPv4).
var dnsAnswerTemplate = []byte{0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x0E, 0x10, 0x00, 0x04, 0, 0, 0, 0}

// parseDNSQuestion returns the query name, type, class and the offset after
// the (first) question.
func parseDNSQuestion(q []byte) (name string, qtype, qclass uint16, end int, err error) {
	if len(q) < 12 {
		return "", 0, 0, 0, errors.New("short packet")
	}
	var labels []string
	i := 12
	for {
		if i >= len(q) {
			return "", 0, 0, 0, errors.New("bad name")
		}
		l := int(q[i])
		if l == 0 {
			i++
			break
		}
		if l&0xC0 != 0 || i+1+l > len(q) {
			return "", 0, 0, 0, errors.New("bad label")
		}
		labels = append(labels, string(q[i+1:i+1+l]))
		i += 1 + l
	}
	if i+4 > len(q) {
		return "", 0, 0, 0, errors.New("short question")
	}
	return strings.Join(labels, "."), binary.BigEndian.Uint16(q[i:]), binary.BigEndian.Uint16(q[i+2:]), i + 4, nil
}

// dnsIntercepted mirrors the InterceptDNs matching of IdDNS_UDPServerDoAfterQuery.
func dnsIntercepted(name string) bool {
	for _, d := range strings.Split(InterceptDNs, ",") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "*") {
			if strings.HasSuffix(strings.ToLower(name), strings.ToLower(d[1:])) {
				return true
			}
		} else if !strings.Contains(d, "*") && name == d {
			return true
		}
	}
	return false
}

// BuildDNSAnswer builds the response for an intercepted query. For standard
// A/IN queries with one question an A record with ip is returned; for other
// query types an empty NOERROR answer (the Pascal version echoed the query).
func BuildDNSAnswer(query []byte, ip net.IP) ([]byte, bool) {
	_, qtype, qclass, end, err := parseDNSQuestion(query)
	if err != nil {
		return nil, false
	}
	opcode := (query[2] >> 3) & 0x0F
	qd := binary.BigEndian.Uint16(query[4:])
	resp := make([]byte, end, end+len(dnsAnswerTemplate))
	copy(resp, query[:end])
	resp[2] = 0x81 // QR=1, RD=1 (flags 0x8180 like the Pascal version)
	resp[3] = 0x80 // RA=1, RCODE=0
	binary.BigEndian.PutUint16(resp[4:], 1)
	binary.BigEndian.PutUint16(resp[8:], 0)  // NSCOUNT
	binary.BigEndian.PutUint16(resp[10:], 0) // ARCOUNT
	ip4 := ip.To4()
	if opcode == 0 && qd == 1 && qtype == 1 && qclass == 1 && ip4 != nil {
		binary.BigEndian.PutUint16(resp[6:], 1)
		ans := append([]byte(nil), dnsAnswerTemplate...)
		copy(ans[12:], ip4)
		return append(resp, ans...), true
	}
	binary.BigEndian.PutUint16(resp[6:], 0)
	return resp, false
}

// forwardDNS sends the query to the upstream servers and returns the first answer.
func forwardDNS(query []byte) ([]byte, error) {
	var lastErr error = errors.New("no upstream DNS servers")
	for _, s := range strings.Split(DNSServers, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		addr := s
		if _, _, err := net.SplitHostPort(s); err != nil {
			addr = net.JoinHostPort(s, "53")
		}
		c, err := net.DialTimeout("udp", addr, 3*time.Second)
		if err != nil {
			lastErr = err
			continue
		}
		_ = c.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err = c.Write(query); err == nil {
			buf := make([]byte, 8192)
			var n int
			if n, err = c.Read(buf); err == nil {
				c.Close()
				return buf[:n], nil
			}
		}
		c.Close()
		lastErr = err
	}
	return nil, lastErr
}

func handleDNSQuery(conn *net.UDPConn, from *net.UDPAddr, query []byte, ip net.IP) {
	name, _, _, _, err := parseDNSQuestion(query)
	if err == nil && dnsIntercepted(name) {
		resp, isA := BuildDNSAnswer(query, ip)
		if resp != nil {
			if isA {
				Logging(LtDebug, "DNS Query intercept : "+name)
			}
			_, _ = conn.WriteToUDP(resp, from)
			return
		}
	}
	resp, err := forwardDNS(query)
	if err != nil {
		Logging(LtDebug, "DNS forward error ("+name+"): "+err.Error())
		return
	}
	_, _ = conn.WriteToUDP(resp, from)
}

// StartDNSServer mirrors StartDNSServer.
func StartDNSServer() bool {
	addr := net.JoinHostPort(DNSServerIPAddress, strconv.Itoa(DNSServerPort))
	ua, err := net.ResolveUDPAddr("udp4", addr)
	if err == nil {
		dnsConn, err = net.ListenUDP("udp4", ua)
	}
	if err != nil {
		Logging(LtError, "DNS Server error: "+err.Error())
		return false
	}
	ip := net.ParseIP(DNSServerIPAddress)
	conn := dnsConn
	go func() {
		buf := make([]byte, 8192)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				continue
			}
			q := append([]byte(nil), buf[:n]...)
			go handleDNSQuery(conn, from, q, ip)
		}
	}()
	return true
}

// StopDNSServer stops the DNS service.
func StopDNSServer() {
	dnsMu.Lock()
	defer dnsMu.Unlock()
	if dnsConn != nil {
		dnsConn.Close()
		dnsConn = nil
	}
}
