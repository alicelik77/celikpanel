//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The Agent's traffic store (alpha.118, D-044): logs are read from where
// they stopped, across logrotate's rename and an Agent restart, and every
// byte is answered in exactly one batch until the Panel confirms it.

func trafficTestDirs(t *testing.T) (logs string) {
	t.Helper()
	logs = filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "store")
	previousStore, previousLogs, previousRotate := trafficStoreDir, trafficLogDir, trafficLogrotatePath
	trafficStoreDir = store
	trafficLogDir = func() string { return logs }
	trafficLogrotatePath = filepath.Join(t.TempDir(), "absent", "celikpanel-traffic")
	trafficLastLogrotate = time.Time{}
	t.Cleanup(func() {
		trafficStoreDir, trafficLogDir, trafficLogrotatePath = previousStore, previousLogs, previousRotate
		trafficLastLogrotate = time.Time{}
	})
	return logs
}

func appendTraffic(t *testing.T, path string, lines ...string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, line := range lines {
		if _, err := file.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
}

func trafficLine(bytes int) string {
	return fmt.Sprintf("%d.123 %d 300\n", time.Now().Unix(), bytes)
}

func collectTraffic(t *testing.T, epoch string, after int64) transport.TrafficCollectResponse {
	t.Helper()
	var resp transport.TrafficCollectResponse
	if err := (&Agent{}).TrafficCollectV1(&transport.TrafficCollectRequest{Epoch: epoch, AfterSeq: after}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("collect: %s", resp.Error)
	}
	return resp
}

func batchBytes(batches []transport.TrafficBatch) (out int64, requests int64) {
	for _, batch := range batches {
		for _, item := range batch.Items {
			out += item.BytesOut
			requests += item.Requests
		}
	}
	return out, requests
}

func TestTrafficStoreFollowsRotationAndRestartWithoutLossOrDoubleCount(t *testing.T) {
	logs := trafficTestDirs(t)
	log := filepath.Join(logs, "7.log")

	// A new store: nothing yet. The first tick makes it.
	if err := trafficTick(); err != nil {
		t.Fatal(err)
	}
	// The site's log appears after the store exists: read from its start.
	appendTraffic(t, log, trafficLine(100), trafficLine(200))
	// An unfinished line is not counted until nginx ends it.
	appendTraffic(t, log, fmt.Sprintf("%d.5 40", time.Now().Unix()))
	if err := trafficTick(); err != nil {
		t.Fatal(err)
	}
	first := collectTraffic(t, "", 0)
	if out, requests := batchBytes(first.Batches); out != 300 || requests != 2 || len(first.Batches) != 1 || first.Batches[0].Seq != 1 {
		t.Fatalf("first collection: %+v", first.Batches)
	}
	epoch := first.Epoch

	// The answer was lost: the same batch comes again, nothing more.
	again := collectTraffic(t, epoch, 0)
	if out, _ := batchBytes(again.Batches); out != 300 || again.Batches[0].Seq != 1 {
		t.Fatalf("repeated collection: %+v", again.Batches)
	}

	// nginx ends the line, then logrotate renames the log, nginx writes a
	// little more to the old file before it reopens, then the new file.
	appendTraffic(t, log, "0 300\n")
	if err := os.Rename(log, log+".1"); err != nil {
		t.Fatal(err)
	}
	appendTraffic(t, log+".1", trafficLine(25))
	appendTraffic(t, log, trafficLine(10))
	if err := trafficTick(); err != nil {
		t.Fatal(err)
	}
	// Late lines in the rotated file are still read (within ten minutes).
	appendTraffic(t, log+".1", trafficLine(5))

	// The Panel confirms batch 1; an Agent restart in between changes
	// nothing (every call reads the store from disk).
	second := collectTraffic(t, epoch, 1)
	out, requests := batchBytes(second.Batches)
	if out != 400+25+10+5 || requests != 4 || len(second.Batches) != 1 || second.Batches[0].Seq != 2 {
		t.Fatalf("after rotation: %d bytes, %d requests, %+v", out, requests, second.Batches)
	}
	if third := collectTraffic(t, epoch, 2); len(third.Batches) != 0 {
		t.Fatalf("confirmed batches answered again: %+v", third.Batches)
	}
	// Another Panel (another epoch) is answered from the store's own start.
	if other := collectTraffic(t, "someone-else", 99); other.Epoch != epoch {
		t.Fatalf("epoch changed: %s", other.Epoch)
	}
}

func TestTrafficStoreStartsExistingLogsAtTheirEndWhenTheStoreIsNew(t *testing.T) {
	logs := trafficTestDirs(t)
	// The log is older than the store (a reinstalled Agent): its bytes may
	// have been counted by the lost store, so they are not counted again.
	appendTraffic(t, filepath.Join(logs, "8.log"), trafficLine(5000))
	if err := trafficTick(); err != nil {
		t.Fatal(err)
	}
	appendTraffic(t, filepath.Join(logs, "8.log"), trafficLine(7))
	resp := collectTraffic(t, "", 0)
	if out, _ := batchBytes(resp.Batches); out != 7 {
		t.Fatalf("counted %d bytes, want only the 7 written after the store was made", out)
	}
}

func TestTrafficStoreRefusesToReplaceAnUnreadableStore(t *testing.T) {
	trafficTestDirs(t)
	if err := os.MkdirAll(trafficStoreDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trafficStorePath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var resp transport.TrafficCollectResponse
	if err := (&Agent{}).TrafficCollectV1(&transport.TrafficCollectRequest{}, &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Error, "not readable") {
		t.Fatalf("error = %q", resp.Error)
	}
	content, _ := os.ReadFile(trafficStorePath())
	if string(content) != "{not json" {
		t.Fatal("an unreadable store was replaced")
	}
}

func TestParseTrafficLineSkipsWhatItCannotRead(t *testing.T) {
	for _, line := range []string{"", "x 1 2", "1730650000.1 -5 2", "1730650000.1 5", "1730650000.1 5 2 9"} {
		if _, _, _, ok := parseTrafficLine(line); ok {
			t.Fatalf("parsed %q", line)
		}
	}
	day, out, in, ok := parseTrafficLine("1730650000.999 512 128")
	if !ok || out != 512 || in != 128 || day != time.Unix(1730650000, 0).In(time.Local).Format("2006-01-02") {
		t.Fatalf("parsed %s %d %d %v", day, out, in, ok)
	}
}

func TestTrafficLogrotateFileRotatesWithoutThePanel(t *testing.T) {
	body := renderTrafficLogrotate()
	for _, want := range []string{"daily", "rotate 3", "maxsize 100M", "delaycompress", "missingok", "systemctl kill --kill-whom=main --signal=USR1 nginx.service"} {
		if !strings.Contains(body, want) {
			t.Fatalf("logrotate file lacks %q:\n%s", want, body)
		}
	}
}
