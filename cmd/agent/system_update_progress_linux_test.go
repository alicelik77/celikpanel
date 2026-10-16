//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The progress read is evidence for the update page's steps (owner design
// 2026-10-16). Each case below fails if the read names more than the
// updater's own files show.

type progressTestFixture struct {
	backend *linuxSystemUpdateBackend
	state   *systemUpdateState
	created time.Time
	base    string
}

func newProgressTestFixture(t *testing.T) progressTestFixture {
	t.Helper()
	base := linuxSystemUpdateTestBase(t)
	created := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	state := linuxSystemUpdateTestState(strings.Repeat("e", 32))
	state.Status = systemUpdateRunning
	state.CreatedAt = created.Format(time.RFC3339Nano)
	state.UpdatedAt = state.CreatedAt
	downloads := filepath.Join(base, "releases")
	if err := os.Mkdir(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := newLinuxSystemUpdateBackend()
	backend.stateRoot = filepath.Join(base, "self-update")
	backend.floorPath = filepath.Join(base, "sequence.floor")
	backend.downloadRoot = downloads
	return progressTestFixture{backend: backend, state: state, created: created, base: base}
}

func (fixture progressTestFixture) writeFloor(t *testing.T, floor systemUpdateFloor, at time.Time) {
	t.Helper()
	if err := os.WriteFile(fixture.backend.floorPath, canonicalSystemUpdateFloor(floor), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fixture.backend.floorPath, at, at); err != nil {
		t.Fatal(err)
	}
}

func (fixture progressTestFixture) writeArchive(t *testing.T, directory string, size int, at time.Time) string {
	t.Helper()
	dir := filepath.Join(fixture.backend.downloadRoot, directory)
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
	path := filepath.Join(dir, systemUpdateArchiveName(fixture.state))
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSystemUpdateProgressReportsTheVerifiedFloorOfThisRequest(t *testing.T) {
	fixture := newProgressTestFixture(t)
	verified := fixture.created.Add(40 * time.Second)
	fixture.writeFloor(t, systemUpdateFloor{Sequence: fixture.state.TargetSequence, Version: fixture.state.TargetVersion}, verified)
	// A finished archive is also there; the floor is the later evidence and wins.
	fixture.writeArchive(t, ".download.AAAAAAAA", 123, fixture.created.Add(30*time.Second))
	progress := fixture.backend.Progress(fixture.state)
	if !progress.VerifiedAt.Equal(verified) {
		t.Fatalf("verified at = %v, want %v", progress.VerifiedAt, verified)
	}
	if progress.HaveBytes || !progress.DownloadedAt.IsZero() {
		t.Fatalf("download reported beside the verified floor: %#v", progress)
	}
	// A succeeded record keeps the same evidence after the restart.
	fixture.state.Status = systemUpdateSucceeded
	if got := fixture.backend.Progress(fixture.state); !got.VerifiedAt.Equal(verified) {
		t.Fatalf("succeeded record lost the verified floor: %#v", got)
	}
}

func TestSystemUpdateProgressIgnoresAFloorThatIsNotThisRequests(t *testing.T) {
	for _, test := range []struct {
		name  string
		floor systemUpdateFloor
		at    time.Duration
	}{
		// A retry of the same release does not rewrite the floor (get.sh
		// persist_release_sequence_floor returns early); its time is older.
		{name: "same release before the request", floor: systemUpdateFloor{Sequence: "42", Version: "v1.2.3"}, at: -time.Hour},
		{name: "previous release", floor: systemUpdateFloor{Sequence: "41", Version: "v1.2.2"}, at: time.Second},
		{name: "other version", floor: systemUpdateFloor{Sequence: "42", Version: "v1.2.4"}, at: time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProgressTestFixture(t)
			fixture.writeFloor(t, test.floor, fixture.created.Add(test.at))
			if got := fixture.backend.Progress(fixture.state); !got.VerifiedAt.IsZero() {
				t.Fatalf("floor counted as this request's verification: %#v", got)
			}
		})
	}
}

func TestSystemUpdateProgressReportsTheArchiveBeingDownloaded(t *testing.T) {
	fixture := newProgressTestFixture(t)
	path := fixture.writeArchive(t, ".download.BBBBBBBB", 50, fixture.created.Add(5*time.Second))
	progress := fixture.backend.Progress(fixture.state)
	if !progress.HaveBytes || progress.DownloadedBytes != 50 || !progress.DownloadedAt.IsZero() {
		t.Fatalf("partial download = %#v", progress)
	}
	complete := fixture.created.Add(9 * time.Second)
	if err := os.WriteFile(path, make([]byte, 123), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, complete, complete); err != nil {
		t.Fatal(err)
	}
	progress = fixture.backend.Progress(fixture.state)
	if !progress.HaveBytes || progress.DownloadedBytes != 123 || !progress.DownloadedAt.Equal(complete) {
		t.Fatalf("complete download = %#v", progress)
	}
	// Only an active record reads the download directory.
	fixture.state.Status = systemUpdateFailed
	if got := fixture.backend.Progress(fixture.state); got.HaveBytes || !got.DownloadedAt.IsZero() {
		t.Fatalf("download read for a finished record: %#v", got)
	}
}

func TestSystemUpdateProgressSaysNothingWhenTheDownloadIsAmbiguous(t *testing.T) {
	cases := map[string]func(*testing.T, progressTestFixture){
		"two candidate directories": func(t *testing.T, fixture progressTestFixture) {
			fixture.writeArchive(t, ".download.CCCCCCCC", 10, fixture.created.Add(time.Second))
			fixture.writeArchive(t, ".download.DDDDDDDD", 20, fixture.created.Add(time.Second))
		},
		"a leftover older than the request": func(t *testing.T, fixture progressTestFixture) {
			fixture.writeArchive(t, ".download.EEEEEEEE", 10, fixture.created.Add(-time.Hour))
		},
		"larger than the signed size": func(t *testing.T, fixture progressTestFixture) {
			fixture.writeArchive(t, ".download.FFFFFFFF", 124, fixture.created.Add(time.Second))
		},
		"a symbolic link": func(t *testing.T, fixture progressTestFixture) {
			target := filepath.Join(fixture.base, "elsewhere.tar.gz")
			if err := os.WriteFile(target, make([]byte, 10), 0o600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(fixture.backend.downloadRoot, ".download.GGGGGGGG")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, systemUpdateArchiveName(fixture.state))); err != nil {
				t.Fatal(err)
			}
		},
		"a directory open to others": func(t *testing.T, fixture progressTestFixture) {
			fixture.writeArchive(t, ".download.HHHHHHHH", 10, fixture.created.Add(time.Second))
			if err := os.Chmod(filepath.Join(fixture.backend.downloadRoot, ".download.HHHHHHHH"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"another name": func(t *testing.T, fixture progressTestFixture) {
			fixture.writeArchive(t, "download.IIIIIIII", 10, fixture.created.Add(time.Second))
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newProgressTestFixture(t)
			arrange(t, fixture)
			if got := fixture.backend.Progress(fixture.state); got.HaveBytes || !got.DownloadedAt.IsZero() || !got.VerifiedAt.IsZero() {
				t.Fatalf("ambiguous download reported: %#v", got)
			}
		})
	}
	// Negative control: the same fixture with one plain candidate is read.
	fixture := newProgressTestFixture(t)
	fixture.writeArchive(t, ".download.JJJJJJJJ", 10, fixture.created.Add(time.Second))
	if got := fixture.backend.Progress(fixture.state); !got.HaveBytes || got.DownloadedBytes != 10 {
		t.Fatalf("control download not read: %#v", got)
	}
}

type progressFakeBackend struct {
	fakeSystemUpdateBackend
	progress systemUpdateProgress
	calls    int
}

func (fake *progressFakeBackend) Progress(*systemUpdateState) systemUpdateProgress {
	fake.calls++
	return fake.progress
}

func TestSystemUpdateStatusRPCCarriesTheProgressAdditively(t *testing.T) {
	state := linuxSystemUpdateTestState(strings.Repeat("f", 32))
	state.Status = systemUpdateRunning
	backend := &progressFakeBackend{
		fakeSystemUpdateBackend: fakeSystemUpdateBackend{status: state},
		progress: systemUpdateProgress{
			DownloadedBytes: 123, HaveBytes: true,
			DownloadedAt: time.Date(2026, 8, 12, 12, 0, 9, 0, time.UTC),
			VerifiedAt:   time.Date(2026, 8, 12, 12, 0, 12, 0, time.UTC),
		},
	}
	service := newSystemUpdateService(nil, backend, "linux", "amd64")
	previousService, previousErr := globalSystemUpdateService, globalSystemUpdateErr
	globalSystemUpdateService, globalSystemUpdateErr = service, nil
	t.Cleanup(func() { globalSystemUpdateService, globalSystemUpdateErr = previousService, previousErr })

	var reply transport.SystemUpdateStatusResponse
	if err := (&Agent{}).SystemUpdateStatus(&transport.SystemUpdateStatusRequest{RequestID: state.RequestID}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Status != systemUpdateRunning || reply.DownloadedBytes != "123" ||
		reply.DownloadedAt != "2026-08-12T12:00:09Z" || reply.VerifiedAt != "2026-08-12T12:00:12Z" {
		t.Fatalf("status reply = %#v", reply)
	}
	// Nothing read means nothing said: the fields stay empty.
	backend.progress = systemUpdateProgress{}
	reply = transport.SystemUpdateStatusResponse{}
	if err := (&Agent{}).SystemUpdateStatus(&transport.SystemUpdateStatusRequest{RequestID: state.RequestID}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.DownloadedBytes != "" || reply.DownloadedAt != "" || reply.VerifiedAt != "" {
		t.Fatalf("empty progress filled the reply: %#v", reply)
	}
	// A record that was not found carries no progress.
	empty := transport.SystemUpdateStatusResponse{}
	applySystemUpdateProgress(&empty, systemUpdateProgress{HaveBytes: true, DownloadedBytes: 1})
	if empty.DownloadedBytes != "" {
		t.Fatalf("progress on a missing record: %#v", empty)
	}
}

// The progress read rests on what the reviewed installer does; this pins it,
// so a change to the installer that moves the evidence fails here first.
func TestSystemUpdateProgressMatchesTheReviewedInstaller(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "download-portal", "get.sh"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{
		"releases_root=" + systemUpdateDownloadRoot + "\n",
		`workdir=$(mktemp -d "$releases_root/.download.XXXXXXXX")`,
		`archive=celikpanel-$version-$runtime_release_os-$runtime_release_arch.tar.gz`,
		`signed_fetch "$release_url/$archive" "$workdir/$archive" "$signed_archive_size"`,
		"release_sequence_floor=" + systemUpdateFloorPath + "\n",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("get.sh no longer contains %q", want)
		}
	}
	order := []string{
		`signed_fetch "$release_url/$archive" "$workdir/$archive"`,
		`(cd "$workdir" && sha256sum -c "$archive.sha256")`,
		`sha256sum -c SHA256SUMS >/dev/null`,
		`persist_release_sequence_floor "$signed_release_sequence" "$version"`,
		`updater=$extracted_root/bootstrap-prebuilt-update.sh`,
	}
	last := -1
	for _, marker := range order {
		index := strings.Index(source, marker)
		if index <= last {
			t.Fatalf("get.sh order changed at %q (index %d after %d)", marker, index, last)
		}
		last = index
	}
}
