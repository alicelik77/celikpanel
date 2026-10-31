package panellisten

import "testing"

func TestDefaultIs1923AndLegacyIs2083(t *testing.T) {
	if DefaultPort != 1923 || DefaultListen != ":1923" || LegacyPort != 2083 {
		t.Fatalf("defaults changed: %d %q %d", DefaultPort, DefaultListen, LegacyPort)
	}
}

func TestPortAndProbeAddress(t *testing.T) {
	for listen, want := range map[string]string{
		":1923":                "127.0.0.1:1923",
		"0.0.0.0:2083":         "127.0.0.1:2083",
		"localhost:1923":       "127.0.0.1:1923",
		"[::]:1923":            "[::1]:1923",
		"[::1]:2083":           "[::1]:2083",
		"192.0.2.10:1923":      "192.0.2.10:1923",
		"panel.example.test:9": "panel.example.test:9",
	} {
		got, err := ProbeAddress(listen)
		if err != nil || got != want {
			t.Fatalf("ProbeAddress(%q) = %q, %v; want %q", listen, got, err, want)
		}
	}
	for _, bad := range []string{"1923", ":0", ":70000", ":x", ""} {
		if _, err := Port(bad); err == nil {
			t.Fatalf("Port(%q) accepted", bad)
		}
	}
	if got, err := WithPort("192.0.2.10:2083", 1923); err != nil || got != "192.0.2.10:1923" {
		t.Fatalf("WithPort keeps the host: %q %v", got, err)
	}
	if got, err := WithPort(":2083", 1923); err != nil || got != ":1923" {
		t.Fatalf("WithPort wildcard: %q %v", got, err)
	}
}

func TestReplaceListenKeepsEveryOtherLine(t *testing.T) {
	raw := []byte("# owner note\nCELIKPANEL_LISTEN=:2083\nCELIKPANEL_TLS=1\nCELIKPANEL_PANEL_INSECURE_COOKIES_FLAG=\nCELIKPANEL_PANEL_DEMO_FLAG=\n")
	out, err := ReplaceListen(raw, ":2083", ":1923")
	if err != nil {
		t.Fatal(err)
	}
	want := "# owner note\nCELIKPANEL_LISTEN=:1923\nCELIKPANEL_TLS=1\nCELIKPANEL_PANEL_INSECURE_COOKIES_FLAG=\nCELIKPANEL_PANEL_DEMO_FLAG=\n"
	if string(out) != want {
		t.Fatalf("got %q", out)
	}
	// The owner changed the file: it no longer says what the panel runs on.
	if _, err := ReplaceListen(raw, ":1923", ":4000"); err == nil {
		t.Fatal("a file saying another port must not be rewritten")
	}
	if _, err := ReplaceListen([]byte("CELIKPANEL_LISTEN=:2083\nCELIKPANEL_LISTEN=:2083\n"), ":2083", ":1923"); err == nil {
		t.Fatal("a repeated key must be refused")
	}
	if _, err := ReplaceListen([]byte("CELIKPANEL_TLS=1\n"), ":2083", ":1923"); err == nil {
		t.Fatal("a file without the key must be refused")
	}
	// No trailing newline.
	out, err = ReplaceListen([]byte("CELIKPANEL_LISTEN=:2083"), ":2083", ":1923")
	if err != nil || string(out) != "CELIKPANEL_LISTEN=:1923" {
		t.Fatalf("no trailing newline: %q %v", out, err)
	}
}

func TestChoiceRefusal(t *testing.T) {
	for _, port := range []int{80, 443, 25, 53, 1023, 65536, 0} {
		if code, _ := ChoiceRefusal(port, nil); code != "PANEL_PORT_RANGE" {
			t.Fatalf("port %d: %q", port, code)
		}
	}
	for _, port := range []int{3306, 5432, 51820} {
		if code, _ := ChoiceRefusal(port, nil); code != "PANEL_PORT_RESERVED" {
			t.Fatalf("port %d: %q", port, code)
		}
	}
	if code, _ := ChoiceRefusal(4000, map[int]string{4000: "x"}); code != "PANEL_PORT_RESERVED" {
		t.Fatal("extra reserved ports are refused")
	}
	for _, port := range []int{1923, 2083, 8443, 65535} {
		if code, reason := ChoiceRefusal(port, nil); code != "" {
			t.Fatalf("port %d refused: %s", port, reason)
		}
	}
}
