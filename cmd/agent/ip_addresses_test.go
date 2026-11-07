package main

import "testing"

func TestParseIPAddrJSONKeepsGlobalAddresses(t *testing.T) {
	raw := []byte(`[
	 {"ifname":"lo","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8,"scope":"host"}]},
	 {"ifname":"eth0","addr_info":[
	   {"family":"inet","local":"10.0.0.5","prefixlen":24,"scope":"global"},
	   {"family":"inet","local":"203.0.113.7","prefixlen":24,"scope":"global"},
	   {"family":"inet6","local":"2001:db8::7","prefixlen":64,"scope":"global"},
	   {"family":"inet6","local":"fe80::1","prefixlen":64,"scope":"link"}]}]`)
	got, err := parseIPAddrJSON(raw)
	if err != nil || len(got) != 3 {
		t.Fatalf("got %+v %v", got, err)
	}
	if got[0].Address != "203.0.113.7" || got[0].Private || got[1].Address != "10.0.0.5" || !got[1].Private || got[2].Family != "ipv6" || got[2].Interface != "eth0" {
		t.Fatalf("order or facts wrong: %+v", got)
	}
	if _, err := parseIPAddrJSON([]byte("not json")); err == nil {
		t.Fatal("garbage parsed")
	}
}
