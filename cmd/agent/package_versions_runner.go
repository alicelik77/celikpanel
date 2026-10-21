package main

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
)

// packageVersionEnvironment is the whole environment of the version query:
// a fixed locale and search path and nothing inherited, so neither a caller's
// locale nor a DPKG_ADMINDIR, DPKG_ROOT or similar variable of the Agent's own
// environment can change which database is read or how it is printed.
// Sorgunun tüm ortamı: sabit yerel ayar ve arama yolu, devralınan hiçbir şey.
var packageVersionEnvironment = []string{
	"LC_ALL=C",
	"LANG=C",
	"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
}

var errPackageVersionOutputTooLarge = errors.New("package database answer is larger than the read ceiling")

// runPackageVersionCommand runs the read-only query with a hard output
// ceiling. The process gets its own process group (killed as a whole on
// cancellation, with a bounded wait for its pipes); the moment its standard
// output passes `limit` bytes the context is cancelled, the group is killed,
// the rest of the pipe is drained without being kept, and the answer is
// errPackageVersionOutputTooLarge, never a partial answer. Standard error is
// discarded: it is never returned to the Panel.
// Sorgu, sabit çıktı tavanıyla çalışır: tavan aşılınca süreç grubu öldürülür,
// boru boşaltılır ve yanıt "çok büyük" hatasıdır, asla yarım bir yanıt değil.
func runPackageVersionCommand(parent context.Context, limit int, executable string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = append([]string(nil), packageVersionEnvironment...)
	cmd.Dir = "/"
	cmd.Stdin = nil
	configureServiceMutationProcessGroup(cmd)
	output := &packageVersionOutput{limit: limit, exceed: cancel}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if output.exceeded() {
		return nil, errPackageVersionOutputTooLarge
	}
	if err != nil {
		return nil, err
	}
	return output.bytes(), nil
}

// packageVersionOutput keeps at most `limit` bytes and reports the overflow
// once; later writes are accepted and dropped so the pipe never blocks.
type packageVersionOutput struct {
	mu     sync.Mutex
	limit  int
	data   []byte
	over   bool
	exceed func()
}

func (o *packageVersionOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.over {
		return len(p), nil
	}
	if len(o.data)+len(p) > o.limit {
		o.over = true
		o.data = nil
		o.exceed()
		return len(p), nil
	}
	o.data = append(o.data, p...)
	return len(p), nil
}

func (o *packageVersionOutput) exceeded() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.over
}

func (o *packageVersionOutput) bytes() []byte {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]byte(nil), o.data...)
}
