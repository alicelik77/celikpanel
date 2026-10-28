package dnswire

import (
	"context"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnswire/dnswiretest"
)

// A recursive resolver and a parent-zone server, both on loopback.
func delegationFixture(t *testing.T, parent string, child dnswiretest.Handler) (DelegationOptions, *dnswiretest.Server) {
	t.Helper()
	parentServer := dnswiretest.Start(t, child)
	resolver := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		if !recursive {
			return dnswiretest.Reply{RCode: 5}
		}
		soa := dnswiretest.RR{Name: parent, Type: dnswiretest.TypeSOA, Value: "ns.nic." + parent + " hostmaster." + parent + " 7 3600 600 604800 300"}
		switch {
		case qtype == dnswiretest.TypeSOA && name == parent:
			return dnswiretest.Reply{Answer: []dnswiretest.RR{soa}}
		case qtype == dnswiretest.TypeSOA:
			return dnswiretest.Reply{Authority: []dnswiretest.RR{soa}}
		case qtype == dnswiretest.TypeNS && name == parent:
			return dnswiretest.Reply{Answer: []dnswiretest.RR{{Name: parent, Type: dnswiretest.TypeNS, Value: "ns.nic." + parent}}}
		case qtype == dnswiretest.TypeA && name == "ns.nic."+parent:
			return dnswiretest.Reply{Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeA, Value: "127.0.0.1"}}}
		}
		return dnswiretest.Reply{RCode: 3}
	})
	return DelegationOptions{Resolvers: []string{resolver.Address}, Address: func(string) string { return parentServer.Address }}, parentServer
}

func TestRegistrarDelegationReadsTheParentWithoutRecursion(t *testing.T) {
	options, parent := delegationFixture(t, "com.tr", func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		if recursive || name != "example.com.tr" || qtype != dnswiretest.TypeNS {
			return dnswiretest.Reply{RCode: 5}
		}
		return dnswiretest.Reply{
			Authority:  []dnswiretest.RR{{Name: "example.com.tr", Type: dnswiretest.TypeNS, Value: "ns1.example.com.tr"}, {Name: "example.com.tr", Type: dnswiretest.TypeNS, Value: "NS2.example.com.tr"}},
			Additional: []dnswiretest.RR{{Name: "ns1.example.com.tr", Type: dnswiretest.TypeA, Value: "203.0.113.10"}, {Name: "ns1.example.com.tr", Type: dnswiretest.TypeAAAA, Value: "2001:db8::10"}, {Name: "evil.test", Type: dnswiretest.TypeA, Value: "192.0.2.66"}},
		}
	})
	result, err := Delegation(context.Background(), "Example.com.tr.", options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Parent != "com.tr" || !result.Registered || !slices.Equal(result.NS, []string{"ns1.example.com.tr", "ns2.example.com.tr"}) {
		t.Fatalf("delegation: %+v", result)
	}
	if !slices.Equal(result.Glue["ns1.example.com.tr"], []string{"203.0.113.10", "2001:db8::10"}) || len(result.Glue["evil.test"]) != 0 || len(result.Glue["ns2.example.com.tr"]) != 0 {
		t.Fatalf("glue must be the in-delegation addresses only: %+v", result.Glue)
	}
	if !slices.Contains(parent.Questions(), "example.com.tr/2") {
		t.Fatalf("the parent was not asked: %v", parent.Questions())
	}
}

func TestRegistrarDelegationUnregisteredAndUnreachable(t *testing.T) {
	options, _ := delegationFixture(t, "test", func(string, uint16, bool) dnswiretest.Reply { return dnswiretest.Reply{RCode: 3} })
	result, err := Delegation(context.Background(), "nothere.test", options)
	if err != nil || result.Registered || result.Parent != "test" {
		t.Fatalf("NXDOMAIN at the parent is a verified answer: %+v %v", result, err)
	}
	options.Address = func(string) string { return "127.0.0.1:1" }
	if _, err := Delegation(context.Background(), "nothere.test", options); err == nil {
		t.Fatal("an unreachable parent must be an error, never an empty delegation")
	}
	if _, err := Delegation(context.Background(), "test", options); err == nil {
		t.Fatal("a name without a parent must be refused")
	}
}

func TestRegistrarDelegationRefusesHostileReplies(t *testing.T) {
	base, id, err := buildQuery("example.test")
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint16(base[len(base)-4:len(base)-2], TypeNS)
	reply := func(flags uint16, counts [3]uint16, tail []byte) []byte {
		message := append([]byte{}, base...)
		binary.BigEndian.PutUint16(message[2:4], flags)
		binary.BigEndian.PutUint16(message[6:8], counts[0])
		binary.BigEndian.PutUint16(message[8:10], counts[1])
		binary.BigEndian.PutUint16(message[10:12], counts[2])
		return append(message, tail...)
	}
	pointerLoop := []byte{0xc0, byte(len(base)), 0, 2, 0, 1, 0, 0, 0, 1, 0, 0}
	for label, message := range map[string][]byte{
		"not a response":   reply(0x0000, [3]uint16{}, nil),
		"truncated":        reply(0x8200, [3]uint16{}, nil),
		"too many records": reply(0x8000, [3]uint16{200, 100, 0}, nil),
		"record missing":   reply(0x8000, [3]uint16{1, 0, 0}, nil),
		"pointer loop":     reply(0x8000, [3]uint16{1, 0, 0}, pointerLoop),
		"trailing data":    reply(0x8000, [3]uint16{}, []byte{1, 2, 3}),
	} {
		if _, err := ParseMessage(message, id, "example.test", TypeNS); err == nil {
			t.Fatalf("%s accepted", label)
		}
	}
	if _, err := ParseMessage(reply(0x8000, [3]uint16{}, nil), id+1, "example.test", TypeNS); err == nil {
		t.Fatal("a foreign identity accepted")
	}
	if message, err := ParseMessage(reply(0x8403, [3]uint16{}, nil), id, "example.test", TypeNS); err != nil || message.RCode != 3 || !message.Authoritative {
		t.Fatalf("authoritative NXDOMAIN: %+v %v", message, err)
	}
	if _, err := Exchange(context.Background(), "not-an-ip:53", "example.test", TypeNS, false); err == nil {
		t.Fatal("a server name instead of a literal address accepted")
	}
	server := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		return dnswiretest.Reply{Authoritative: !recursive, Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeSOA, Value: "ns1.example.test hostmaster.example.test 2026102801 3600 600 604800 300"}}}
	})
	message, err := Exchange(context.Background(), server.Address, "example.test", TypeSOA, false)
	if err != nil || !message.Authoritative || len(message.Answer) != 1 {
		t.Fatalf("SOA exchange: %+v %v", message, err)
	}
	if serial, ok := SOASerial(message.Answer[0].Value); !ok || serial != 2026102801 {
		t.Fatalf("serial: %d %v", serial, ok)
	}
}
