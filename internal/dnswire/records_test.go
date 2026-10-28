package dnswire

import (
	"encoding/binary"
	"errors"
	"net"
	"slices"
	"testing"
)

func recordAnswer(t *testing.T, name string, qtype uint16, rcode uint16, add func(add func(owner string, kind uint16, data []byte))) ([]byte, uint16) {
	t.Helper()
	message, id, err := buildQuery(name)
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint16(message[len(message)-4:len(message)-2], qtype)
	binary.BigEndian.PutUint16(message[2:4], 0x8180|rcode)
	count := 0
	add(func(owner string, kind uint16, data []byte) {
		encoded, _ := encodeDNSName(owner)
		message = append(message, encoded...)
		message = append(message, byte(kind>>8), byte(kind), 0, 1, 0, 0, 0, 60, byte(len(data)>>8), byte(len(data)))
		message = append(message, data...)
		count++
	})
	binary.BigEndian.PutUint16(message[6:8], uint16(count))
	return message, id
}

func hostData(host string) []byte { encoded, _ := encodeDNSName(host); return encoded }

func TestRecordsDecodeEachTypeAtTheAskedOwner(t *testing.T) {
	cases := []struct {
		qtype uint16
		owner string
		data  []byte
		want  string
	}{
		{TypeMX, "example.test", append([]byte{0, 10}, hostData("Mail.Example.Test")...), "10 mail.example.test"},
		{TypeTXT, "example.test", append(append([]byte{6}, "v=spf1"...), append([]byte{5}, " -all"...)...), "v=spf1 -all"},
		{TypeCAA, "example.test", append([]byte{0, 5}, []byte("issueletsencrypt.org")...), `0 issue "letsencrypt.org"`},
		{TypeNS, "example.test", hostData("ns1.example.test"), "ns1.example.test"},
		{TypeAAAA, "example.test", net.ParseIP("2001:db8::1").To16(), "2001:db8::1"},
	}
	for _, test := range cases {
		message, id := recordAnswer(t, "example.test", test.qtype, 0, func(add func(string, uint16, []byte)) {
			add(test.owner, test.qtype, test.data)
			add("other.test", test.qtype, test.data)
		})
		got, err := ParseRecords(message, id, "example.test", test.qtype)
		if err != nil || !slices.Equal(got, []string{test.want}) {
			t.Fatalf("type %d: %v %v", test.qtype, got, err)
		}
	}
}

func TestRecordsFollowAliasesAndSeparateMissingFromEmpty(t *testing.T) {
	message, id := recordAnswer(t, "www.example.test", TypeA, 0, func(add func(string, uint16, []byte)) {
		add("www.example.test", TypeCNAME, hostData("example.test"))
		add("example.test", TypeA, net.ParseIP("192.0.2.80").To4())
	})
	if got, err := ParseRecords(message, id, "www.example.test", TypeA); err != nil || !slices.Equal(got, []string{"192.0.2.80"}) {
		t.Fatalf("alias chain: %v %v", got, err)
	}
	cname, cid := recordAnswer(t, "www.example.test", TypeCNAME, 0, func(add func(string, uint16, []byte)) {
		add("www.example.test", TypeCNAME, hostData("Example.test"))
	})
	if got, err := ParseRecords(cname, cid, "www.example.test", TypeCNAME); err != nil || !slices.Equal(got, []string{"example.test"}) {
		t.Fatalf("cname: %v %v", got, err)
	}
	empty, eid := recordAnswer(t, "example.test", TypeMX, 0, func(func(string, uint16, []byte)) {})
	if got, err := ParseRecords(empty, eid, "example.test", TypeMX); err != nil || len(got) != 0 {
		t.Fatalf("no data must be an empty answer: %v %v", got, err)
	}
	missing, mid := recordAnswer(t, "nope.example.test", TypeA, 3, func(func(string, uint16, []byte)) {})
	var dnsErr *net.DNSError
	if _, err := ParseRecords(missing, mid, "nope.example.test", TypeA); !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
		t.Fatalf("NXDOMAIN must be not-found: %v", err)
	}
	failed, fid := recordAnswer(t, "example.test", TypeA, 2, func(func(string, uint16, []byte)) {})
	if _, err := ParseRecords(failed, fid, "example.test", TypeA); err == nil || errors.As(err, &dnsErr) {
		t.Fatalf("SERVFAIL must be a failure, not missing: %v", err)
	}
}
