package main

import (
	"bytes"
	"strings"
	"testing"
)

// alpha.124 (Frankfurt): an unknown word must never start a full process.
func TestPanelCommandLineAnswersVersionAndRefusesBareWords(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-version"}, {"version"}} {
		var out bytes.Buffer
		if !emitPanelVersion(args, &out) || !strings.HasPrefix(out.String(), "CelikPanel panel ") {
			t.Fatalf("%v: %q", args, out.String())
		}
	}
	var out bytes.Buffer
	if emitPanelVersion([]string{"--version", "x"}, &out) || emitPanelVersion(nil, &out) || out.Len() != 0 {
		t.Fatal("not the version mode")
	}
	if err := panelPositionalArguments(nil); err != nil {
		t.Fatal(err)
	}
	if err := panelPositionalArguments([]string{"status"}); err == nil || !strings.Contains(err.Error(), "nothing was started") {
		t.Fatalf("a bare word was accepted: %v", err)
	}
}
