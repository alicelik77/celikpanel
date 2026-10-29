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

// Record types read by QueryRecords (setup's public-record reading, alpha.112).
const (
	TypeNS    uint16 = 2
	TypeCNAME uint16 = 5
	TypeMX    uint16 = 15
	TypeTXT   uint16 = 16
	TypeCAA   uint16 = 257
)

// QueryRecords asks exactly the supplied literal resolver one question over
// bounded DNS/TCP and returns the answer's values in a stable text form:
// A/AAAA an address, MX "preference host", TXT the joined strings, CNAME and
// NS a host, CAA "flags tag value". A name that does not exist is a
// net.DNSError with IsNotFound; an existing name without that type is an
// empty answer. Like Query it never uses hosts/NSS or search domains.
// QueryRecords: tek cozumleyiciye tek soru; degerler kararli metin bicimiyle.
func QueryRecords(ctx context.Context, resolver, name string, qtype uint16) ([]string, error) {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	host, port, err := net.SplitHostPort(resolver)
	number, portErr := strconv.Atoi(port)
	if err != nil || net.ParseIP(host) == nil || portErr != nil || number < 1 || number > 65535 {
		return nil, errors.New("DNS resolver must be a literal IP and port")
	}
	switch qtype {
	case TypeA, TypeAAAA, TypeNS, TypeCNAME, TypeMX, TypeTXT, TypeCAA:
	default:
		return nil, errors.New("unsupported DNS question")
	}
	query, id, err := buildQuery(name)
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint16(query[2:4], 0x0100)
	binary.BigEndian.PutUint16(query[len(query)-4:len(query)-2], qtype)
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", resolver)
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
	if length < 12 || length > 65535 {
		return nil, errors.New("DNS response exceeds its bounded frame")
	}
	message := make([]byte, length)
	if _, err := io.ReadFull(conn, message); err != nil {
		return nil, err
	}
	return ParseRecords(message, id, name, qtype)
}

// ParseRecords decodes one answer for QueryRecords. Only data at the exact
// owner, or at the end of its alias chain, answers the question; a CNAME
// question returns the alias target itself.
func ParseRecords(message []byte, id uint16, name string, qtype uint16) ([]string, error) {
	if len(message) < 12 || binary.BigEndian.Uint16(message[:2]) != id {
		return nil, errors.New("DNS response identity differs")
	}
	flags := binary.BigEndian.Uint16(message[2:4])
	if flags&0x8000 == 0 || flags&0x7a00 != 0 || binary.BigEndian.Uint16(message[4:6]) != 1 {
		return nil, errors.New("DNS response is not a complete successful answer")
	}
	question, next, err := decodeDNSName(message, 12)
	if err != nil || next+4 > len(message) || strings.ToLower(strings.TrimSuffix(question, ".")) != name || binary.BigEndian.Uint16(message[next:next+2]) != qtype || binary.BigEndian.Uint16(message[next+2:next+4]) != classIN {
		return nil, errors.New("DNS response question differs")
	}
	switch flags & 15 {
	case 0:
	case 3:
		return nil, &net.DNSError{Err: "DNS name does not exist", Name: name, IsNotFound: true}
	default:
		return nil, fmt.Errorf("DNS resolver returned response code %d", flags&15)
	}
	offset := next + 4
	answers := int(binary.BigEndian.Uint16(message[6:8]))
	if answers > 256 {
		return nil, errors.New("DNS response has too many records")
	}
	values := map[string][]string{}
	aliases := map[string]string{}
	for index := 0; index < answers; index++ {
		owner, header, err := decodeDNSName(message, offset)
		if err != nil || header+10 > len(message) {
			return nil, errors.New("DNS response record is malformed")
		}
		typeID := binary.BigEndian.Uint16(message[header : header+2])
		class := binary.BigEndian.Uint16(message[header+2 : header+4])
		start := header + 10
		end := start + int(binary.BigEndian.Uint16(message[header+8:header+10]))
		if end > len(message) {
			return nil, errors.New("DNS response record is truncated")
		}
		offset = end
		owner = strings.ToLower(strings.TrimSuffix(owner, "."))
		if class != classIN {
			continue
		}
		if typeID == TypeCNAME {
			target, after, err := decodeDNSName(message, start)
			if err != nil || after != end {
				return nil, errors.New("DNS alias is malformed")
			}
			target = strings.ToLower(strings.TrimSuffix(target, "."))
			if previous, exists := aliases[owner]; exists && previous != target {
				return nil, errors.New("DNS alias has conflicting targets")
			}
			aliases[owner] = target
			if qtype != TypeCNAME {
				continue
			}
		}
		if typeID != qtype {
			continue
		}
		value, err := recordValue(message, start, end, typeID)
		if err != nil {
			return nil, err
		}
		values[owner] = append(values[owner], value)
	}
	// The records after the answer section are not read here, but a reply
	// that claims more data than it carries is refused as ParseResponse does.
	total := answers + int(binary.BigEndian.Uint16(message[8:10])) + int(binary.BigEndian.Uint16(message[10:12]))
	for index := answers; index < total; index++ {
		_, header, err := decodeDNSName(message, offset)
		if err != nil || header+10 > len(message) {
			return nil, errors.New("DNS response record is malformed")
		}
		end := header + 10 + int(binary.BigEndian.Uint16(message[header+8:header+10]))
		if end > len(message) {
			return nil, errors.New("DNS response record is truncated")
		}
		offset = end
	}
	if offset != len(message) || total > 256 {
		return nil, errors.New("DNS response has trailing data")
	}
	if qtype == TypeCNAME {
		return values[name], nil
	}
	owner := name
	for depth := 0; depth < 16; depth++ {
		target, alias := aliases[owner]
		if !alias {
			return values[owner], nil
		}
		owner = target
	}
	return nil, errors.New("DNS alias chain exceeds its limit")
}

func recordValue(message []byte, start, end int, typeID uint16) (string, error) {
	data := message[start:end]
	switch typeID {
	case TypeA:
		if len(data) != 4 {
			return "", errors.New("DNS address is malformed")
		}
		return net.IP(data).String(), nil
	case TypeAAAA:
		if len(data) != 16 {
			return "", errors.New("DNS address is malformed")
		}
		return net.IP(data).String(), nil
	case TypeNS, TypeCNAME:
		host, after, err := decodeDNSName(message, start)
		if err != nil || after != end {
			return "", errors.New("DNS host name is malformed")
		}
		return strings.ToLower(strings.TrimSuffix(host, ".")), nil
	case TypeMX:
		if len(data) < 3 {
			return "", errors.New("DNS mail exchanger is malformed")
		}
		host, after, err := decodeDNSName(message, start+2)
		if err != nil || after != end {
			return "", errors.New("DNS mail exchanger is malformed")
		}
		return fmt.Sprintf("%d %s", binary.BigEndian.Uint16(data[:2]), strings.ToLower(strings.TrimSuffix(host, "."))), nil
	case TypeTXT:
		var text strings.Builder
		for index := 0; index < len(data); {
			size := int(data[index])
			if index+1+size > len(data) {
				return "", errors.New("DNS text is malformed")
			}
			text.Write(data[index+1 : index+1+size])
			index += 1 + size
		}
		return text.String(), nil
	case TypeCAA:
		if len(data) < 2 || 2+int(data[1]) > len(data) {
			return "", errors.New("DNS CAA record is malformed")
		}
		tag := string(data[2 : 2+int(data[1])])
		return fmt.Sprintf("%d %s %q", data[0], tag, string(data[2+int(data[1]):])), nil
	}
	return "", errors.New("unsupported DNS record type")
}
