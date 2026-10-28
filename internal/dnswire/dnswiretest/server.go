// Package dnswiretest is a loopback DNS server for tests: it answers DNS/TCP
// and DNS/UDP on 127.0.0.1 with whatever a test's handler returns, so code
// that asks resolvers and authoritative servers is tested without any real
// network. It is never used by production code.
package dnswiretest

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	TypeA     uint16 = 1
	TypeNS    uint16 = 2
	TypeCNAME uint16 = 5
	TypeSOA   uint16 = 6
	TypeMX    uint16 = 15
	TypeTXT   uint16 = 16
	TypeAAAA  uint16 = 28
)

// RR is one record in the text form dnswire returns.
type RR struct {
	Name  string
	Type  uint16
	Value string
}

// Reply is a handler's answer.
type Reply struct {
	RCode         int
	Authoritative bool
	Answer        []RR
	Authority     []RR
	Additional    []RR
}

// Handler answers one question; recursive tells whether recursion was asked.
type Handler func(name string, qtype uint16, recursive bool) Reply

// Server is one loopback DNS server.
type Server struct {
	Address string
	mu      sync.Mutex
	Asked   []string
}

// Start serves handler on 127.0.0.1 over TCP and UDP on the same port until
// the test ends.
func Start(t testing.TB, handler Handler) *Server {
	t.Helper()
	var tcp net.Listener
	var udp net.PacketConn
	for attempt := 0; attempt < 20; attempt++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		packet, err := net.ListenPacket("udp", listener.Addr().String())
		if err != nil {
			listener.Close()
			continue
		}
		tcp, udp = listener, packet
		break
	}
	if tcp == nil {
		t.Fatal("no loopback port for TCP and UDP")
	}
	server := &Server{Address: tcp.Addr().String()}
	t.Cleanup(func() { tcp.Close(); udp.Close() })
	go func() {
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var size [2]byte
				if _, err := io.ReadFull(conn, size[:]); err != nil {
					return
				}
				query := make([]byte, binary.BigEndian.Uint16(size[:]))
				if _, err := io.ReadFull(conn, query); err != nil {
					return
				}
				reply, err := server.answer(query, handler)
				if err != nil {
					return
				}
				frame := make([]byte, 2+len(reply))
				binary.BigEndian.PutUint16(frame, uint16(len(reply)))
				copy(frame[2:], reply)
				_, _ = conn.Write(frame)
			}(conn)
		}
	}()
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, from, err := udp.ReadFrom(buffer)
			if err != nil {
				return
			}
			if reply, err := server.answer(append([]byte{}, buffer[:n]...), handler); err == nil {
				_, _ = udp.WriteTo(reply, from)
			}
		}
	}()
	return server
}

// Port is the server's port.
func (s *Server) Port() string { _, port, _ := net.SplitHostPort(s.Address); return port }

// Questions lists "name/type" asked so far.
func (s *Server) Questions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.Asked...)
}

func (s *Server) answer(query []byte, handler Handler) ([]byte, error) {
	if len(query) < 17 {
		return nil, errors.New("short query")
	}
	name, next, err := readName(query, 12)
	if err != nil || next+4 > len(query) {
		return nil, errors.New("bad question")
	}
	qtype := binary.BigEndian.Uint16(query[next : next+2])
	recursive := query[2]&0x01 != 0
	s.mu.Lock()
	s.Asked = append(s.Asked, name+"/"+strconv.Itoa(int(qtype)))
	s.mu.Unlock()
	reply := handler(name, qtype, recursive)
	out := append([]byte{}, query[:next+4]...)
	flags := uint16(0x8000) | uint16(reply.RCode&15)
	if recursive {
		flags |= 0x0100 | 0x0080
	}
	if reply.Authoritative {
		flags |= 0x0400
	}
	binary.BigEndian.PutUint16(out[2:4], flags)
	binary.BigEndian.PutUint16(out[6:8], uint16(len(reply.Answer)))
	binary.BigEndian.PutUint16(out[8:10], uint16(len(reply.Authority)))
	binary.BigEndian.PutUint16(out[10:12], uint16(len(reply.Additional)))
	for _, section := range [][]RR{reply.Answer, reply.Authority, reply.Additional} {
		for _, rr := range section {
			encoded, err := encodeRR(rr)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded...)
		}
	}
	return out, nil
}

func readName(message []byte, offset int) (string, int, error) {
	labels := []string{}
	for offset < len(message) {
		length := int(message[offset])
		offset++
		if length == 0 {
			return strings.ToLower(strings.Join(labels, ".")), offset, nil
		}
		if length > 63 || offset+length > len(message) {
			return "", 0, errors.New("bad name")
		}
		labels = append(labels, string(message[offset:offset+length]))
		offset += length
	}
	return "", 0, errors.New("bad name")
}

func encodeName(name string) []byte {
	out := []byte{}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" {
			continue
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0)
}

func encodeRR(rr RR) ([]byte, error) {
	var data []byte
	switch rr.Type {
	case TypeA, TypeAAAA:
		ip := net.ParseIP(rr.Value)
		if ip == nil {
			return nil, errors.New("bad address")
		}
		if rr.Type == TypeA {
			data = ip.To4()
		} else {
			data = ip.To16()
		}
	case TypeNS, TypeCNAME:
		data = encodeName(rr.Value)
	case TypeMX:
		fields := strings.Fields(rr.Value)
		preference, _ := strconv.Atoi(fields[0])
		data = append([]byte{byte(preference >> 8), byte(preference)}, encodeName(fields[1])...)
	case TypeTXT:
		value := rr.Value
		for len(value) > 255 {
			data = append(append(data, 255), value[:255]...)
			value = value[255:]
		}
		data = append(append(data, byte(len(value))), value...)
	case TypeSOA:
		fields := strings.Fields(rr.Value)
		if len(fields) != 7 {
			return nil, errors.New("bad SOA")
		}
		data = append(encodeName(fields[0]), encodeName(fields[1])...)
		for _, number := range fields[2:] {
			value, _ := strconv.ParseUint(number, 10, 32)
			data = binary.BigEndian.AppendUint32(data, uint32(value))
		}
	default:
		return nil, errors.New("unsupported type")
	}
	out := encodeName(rr.Name)
	out = append(out, byte(rr.Type>>8), byte(rr.Type), 0, 1, 0, 0, 1, 44, byte(len(data)>>8), byte(len(data)))
	return append(out, data...), nil
}
