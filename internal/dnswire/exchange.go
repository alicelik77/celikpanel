package dnswire

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// TypeSOA is the start-of-authority record type.
const TypeSOA uint16 = 6

// RR is one resource record of any section, its value in the stable text
// form of QueryRecords (SOA: "mname rname serial refresh retry expire minimum").
type RR struct {
	Name  string
	Type  uint16
	TTL   uint32
	Value string
}

// Message is a decoded answer with all three sections. Records of a type
// this package does not read are skipped, never guessed.
type Message struct {
	RCode         int
	Authoritative bool
	Answer        []RR
	Authority     []RR
	Additional    []RR
}

// Exchange asks exactly one literal server one question over bounded DNS/TCP
// (alpha.113, the registrar check). recursive=false asks an authoritative
// server for its own data, as a parent zone's server is asked for a
// delegation. The reply is bound to the question and its size, record count,
// names and lengths are all checked; a hostile or malformed reply is an error.
// Exchange: tek sunucuya tek soru, ozyinelemesiz secenekle; tum bolumler.
func Exchange(ctx context.Context, server, name string, qtype uint16, recursive bool) (*Message, error) {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	host, port, err := net.SplitHostPort(server)
	number, portErr := strconv.Atoi(port)
	if err != nil || net.ParseIP(host) == nil || portErr != nil || number < 1 || number > 65535 {
		return nil, errors.New("DNS server must be a literal IP and port")
	}
	query, id, err := buildQuery(name)
	if err != nil {
		return nil, err
	}
	if recursive {
		binary.BigEndian.PutUint16(query[2:4], 0x0100)
	}
	binary.BigEndian.PutUint16(query[len(query)-4:len(query)-2], qtype)
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", server)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, _ := probeCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	frame := make([]byte, len(query)+2)
	binary.BigEndian.PutUint16(frame, uint16(len(query)))
	copy(frame[2:], query)
	if _, err := conn.Write(frame); err != nil {
		return nil, err
	}
	var size [2]byte
	if _, err := io.ReadFull(conn, size[:]); err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint16(size[:]))
	if length < 12 {
		return nil, errors.New("DNS response is too short")
	}
	message := make([]byte, length)
	if _, err := io.ReadFull(conn, message); err != nil {
		return nil, err
	}
	return ParseMessage(message, id, name, qtype)
}

// ParseMessage decodes a reply to (name, qtype) with id.
func ParseMessage(message []byte, id uint16, name string, qtype uint16) (*Message, error) {
	if len(message) < 12 || binary.BigEndian.Uint16(message[:2]) != id {
		return nil, errors.New("DNS response identity differs")
	}
	flags := binary.BigEndian.Uint16(message[2:4])
	if flags&0x8000 == 0 || flags&0x7800 != 0 || flags&0x0200 != 0 || binary.BigEndian.Uint16(message[4:6]) != 1 {
		return nil, errors.New("DNS response is not a complete answer")
	}
	question, next, err := decodeDNSName(message, 12)
	if err != nil || next+4 > len(message) || strings.ToLower(strings.TrimSuffix(question, ".")) != name || binary.BigEndian.Uint16(message[next:next+2]) != qtype || binary.BigEndian.Uint16(message[next+2:next+4]) != classIN {
		return nil, errors.New("DNS response question differs")
	}
	result := &Message{RCode: int(flags & 15), Authoritative: flags&0x0400 != 0}
	counts := []int{int(binary.BigEndian.Uint16(message[6:8])), int(binary.BigEndian.Uint16(message[8:10])), int(binary.BigEndian.Uint16(message[10:12]))}
	if counts[0]+counts[1]+counts[2] > 256 {
		return nil, errors.New("DNS response has too many records")
	}
	offset := next + 4
	sections := []*[]RR{&result.Answer, &result.Authority, &result.Additional}
	for section, count := range counts {
		for index := 0; index < count; index++ {
			owner, header, err := decodeDNSName(message, offset)
			if err != nil || header+10 > len(message) {
				return nil, errors.New("DNS response record is malformed")
			}
			typeID := binary.BigEndian.Uint16(message[header : header+2])
			class := binary.BigEndian.Uint16(message[header+2 : header+4])
			ttl := binary.BigEndian.Uint32(message[header+4 : header+8])
			start := header + 10
			end := start + int(binary.BigEndian.Uint16(message[header+8:header+10]))
			if end > len(message) {
				return nil, errors.New("DNS response record is truncated")
			}
			offset = end
			if class != classIN {
				continue
			}
			var value string
			switch typeID {
			case TypeSOA:
				value, err = soaValue(message, start, end)
			case TypeA, TypeAAAA, TypeNS, TypeCNAME, TypeMX, TypeTXT, TypeCAA:
				value, err = recordValue(message, start, end, typeID)
			default:
				continue
			}
			if err != nil {
				return nil, err
			}
			*sections[section] = append(*sections[section], RR{Name: strings.ToLower(strings.TrimSuffix(owner, ".")), Type: typeID, TTL: ttl, Value: value})
		}
	}
	if offset != len(message) {
		return nil, errors.New("DNS response has trailing data")
	}
	return result, nil
}

func soaValue(message []byte, start, end int) (string, error) {
	mname, next, err := decodeDNSName(message, start)
	if err != nil {
		return "", errors.New("DNS SOA is malformed")
	}
	rname, next, err := decodeDNSName(message, next)
	if err != nil || next+20 != end {
		return "", errors.New("DNS SOA is malformed")
	}
	numbers := make([]string, 5)
	for index := range numbers {
		numbers[index] = strconv.FormatUint(uint64(binary.BigEndian.Uint32(message[next+4*index:next+4*index+4])), 10)
	}
	return strings.ToLower(strings.TrimSuffix(mname, ".")) + " " + strings.ToLower(strings.TrimSuffix(rname, ".")) + " " + strings.Join(numbers, " "), nil
}

// SOASerial reads the serial of an SOA value from Message.
func SOASerial(value string) (uint32, bool) {
	fields := strings.Fields(value)
	if len(fields) != 7 {
		return 0, false
	}
	serial, err := strconv.ParseUint(fields[2], 10, 32)
	return uint32(serial), err == nil
}

// DelegationOptions bound and direct the walk. Resolvers are recursive
// resolvers (literal ip:port). Address maps a server's IP to the address
// asked (port 53 unless replaced; tests point it at loopback servers).
type DelegationOptions struct {
	Resolvers []string
	Address   func(ip string) string
}

// DelegationResult is what the parent zone's own servers say about a domain.
type DelegationResult struct {
	Parent     string
	Asked      string
	Registered bool
	NS         []string
	Glue       map[string][]string
}

func (o DelegationOptions) address(ip string) string {
	if o.Address != nil {
		return o.Address(ip)
	}
	return net.JoinHostPort(ip, "53")
}

func (o DelegationOptions) recursive(ctx context.Context, name string, qtype uint16) (*Message, error) {
	var last error = errors.New("no recursive resolver")
	for _, resolver := range o.Resolvers {
		message, err := Exchange(ctx, resolver, name, qtype, true)
		if err == nil && (message.RCode == 0 || message.RCode == 3) {
			return message, nil
		}
		if err == nil {
			err = fmt.Errorf("DNS resolver returned response code %d", message.RCode)
		}
		last = err
	}
	return nil, last
}

// Delegation finds the zone above domain by its SOA (so two-label public
// suffixes such as com.tr are handled by the DNS itself, not by a list),
// then asks that zone's own servers, without recursion, which nameservers
// and glue addresses they hand out for domain: what the registrar's change
// published, before any cache sees it. At most four parent servers are
// asked; the first complete answer wins.
// Delegation: ust bolgenin kendi sunuculari alan adi icin ne veriyor.
func Delegation(ctx context.Context, domain string, options DelegationOptions) (DelegationResult, error) {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	result := DelegationResult{Glue: map[string][]string{}}
	dot := strings.IndexByte(domain, '.')
	if dot <= 0 || dot == len(domain)-1 {
		return result, errors.New("domain has no parent zone")
	}
	soa, err := options.recursive(ctx, domain[dot+1:], TypeSOA)
	if err != nil {
		return result, err
	}
	for _, rr := range append(append([]RR{}, soa.Answer...), soa.Authority...) {
		if rr.Type == TypeSOA && rr.Name != domain && strings.HasSuffix(domain, "."+rr.Name) {
			result.Parent = rr.Name
			break
		}
	}
	if result.Parent == "" {
		return result, errors.New("the parent zone could not be found")
	}
	nsAnswer, err := options.recursive(ctx, result.Parent, TypeNS)
	if err != nil {
		return result, err
	}
	addresses := []string{}
	for _, rr := range nsAnswer.Answer {
		if rr.Type != TypeNS || rr.Name != result.Parent || len(addresses) >= 4 {
			continue
		}
		found, err := options.recursive(ctx, rr.Value, TypeA)
		if err != nil {
			continue
		}
		for _, address := range found.Answer {
			if address.Type == TypeA && len(addresses) < 4 {
				addresses = append(addresses, address.Value)
				break
			}
		}
	}
	if len(addresses) == 0 {
		return result, errors.New("the parent zone's servers could not be found")
	}
	var last error
	for _, address := range addresses {
		message, err := Exchange(ctx, options.address(address), domain, TypeNS, false)
		if err != nil {
			last = err
			continue
		}
		if message.RCode == 3 {
			result.Asked, result.Registered = address, false
			return result, nil
		}
		if message.RCode != 0 {
			last = fmt.Errorf("parent server returned response code %d", message.RCode)
			continue
		}
		ns := []string{}
		for _, rr := range append(append([]RR{}, message.Authority...), message.Answer...) {
			if rr.Type == TypeNS && rr.Name == domain && !contains(ns, rr.Value) && len(ns) < 16 {
				ns = append(ns, rr.Value)
			}
		}
		for _, rr := range message.Additional {
			if (rr.Type == TypeA || rr.Type == TypeAAAA) && contains(ns, rr.Name) && !contains(result.Glue[rr.Name], rr.Value) {
				result.Glue[rr.Name] = append(result.Glue[rr.Name], rr.Value)
			}
		}
		result.Asked, result.Registered, result.NS = address, true, ns
		return result, nil
	}
	if last == nil {
		last = errors.New("no parent server answered")
	}
	return result, last
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
