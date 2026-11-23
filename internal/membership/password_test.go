package membership

// Review of PR #361, finding 5: a burst of sign-ins never runs more password work at once
// than the slots allow; the rest waits its turn and every check still completes.

import (
	"encoding/base64"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPasswordWorkIsBounded(t *testing.T) {
	var active, most atomic.Int32
	passwordWorkHook = func() {
		n := active.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond) // long enough for the burst to overlap
		active.Add(-1)
	}
	defer func() { passwordWorkHook = nil }()
	// A cheap hash in the stored format, so the burst is quick; a real one would only be slower.
	cheap := "$argon2id$v=19$m=8,t=1,p=1$" + base64.RawStdEncoding.EncodeToString([]byte("saltsaltsaltsalt")) + "$" +
		base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	var wg sync.WaitGroup
	done := atomic.Int32{}
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			verifyPassword("burst-password-1234", cheap)
			done.Add(1)
		}()
	}
	wg.Wait()
	if most.Load() > int32(cap(passwordSlots)) || most.Load() < 2 {
		t.Fatalf("%d checks ran at once; the bound is %d", most.Load(), cap(passwordSlots))
	}
	if done.Load() != 16 {
		t.Fatalf("only %d of 16 checks finished", done.Load())
	}
}
