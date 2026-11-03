package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// alpha.117 release-owner decisions on D-043 (a-e), on the emergency
// fixture of security_emergency_test.go.

// a: a Node.js app the emergency stops is also locked at systemd level; the
// lock goes on re-enable, and a lock changed on the server is kept.
func TestEmergencyNodeStopLocksTheAppAndReenableUnlocksIt(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "node", "22.11.0", "").Emergency.ID
	f.step(t, id, "stop", confirmBody("Node.js 22.11.0", ""), http.StatusAccepted, "")
	f.run(t)
	if f.native.appLocks[9930] != "22.11.0" || f.sus.apps[9930].Active {
		t.Fatalf("after stop: locks %v app %+v", f.native.appLocks, f.sus.apps[9930])
	}
	v := f.detail(t, id)
	found := false
	for _, effect := range v.Operation.Effects {
		if effect.Kind == emergencyEffectApp {
			found = strings.Contains(effect.Detail, "systemd refuses to start it on Node.js 22.11.0")
		}
	}
	if !found {
		t.Fatalf("app effect does not name the lock: %+v", v.Operation.Effects)
	}
	f.step(t, id, "reenable", `{"reason":"patched","password":"`+emergencyTestPassword+`"}`, http.StatusAccepted, "")
	f.run(t)
	if _, locked := f.native.appLocks[9930]; locked || !f.sus.apps[9930].Active {
		t.Fatalf("after re-enable: locks %v app %+v", f.native.appLocks, f.sus.apps[9930])
	}
}

func TestEmergencyReenableKeepsAnOwnerEditedLockAndDoesNotStart(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "node", "22.11.0", "").Emergency.ID
	f.step(t, id, "stop", confirmBody("Node.js 22.11.0", ""), http.StatusAccepted, "")
	f.run(t)
	f.native.editedLocks = map[int]bool{9930: true}
	f.step(t, id, "reenable", `{"reason":"patched","password":"`+emergencyTestPassword+`"}`, http.StatusAccepted, "")
	f.run(t)
	if f.sus.apps[9930].Active {
		t.Fatal("the app started past a lock file changed on the server")
	}
	v := f.detail(t, id)
	kept := false
	for _, effect := range v.Operation.Effects {
		if effect.Kind == emergencyEffectApp {
			kept = effect.State == effectKept && strings.Contains(effect.Detail, "changed on the server") && strings.Contains(effect.Detail, "not started")
		}
	}
	if !kept {
		t.Fatalf("the edited lock is not reported as kept: %+v", v.Operation.Effects)
	}
}

// c: the move keeps the lock on the withdrawn version until the new one is
// proven; a failed move stops the app and puts it back without a start.
func TestNodeEmergencyMoveNeverStartsTheWithdrawnVersion(t *testing.T) {
	type world struct {
		calls   []string
		locked  bool
		running string // version the app runs on, "" stopped
	}
	build := func(w *world, moveErr, lockKept, stopErr, backErr error) nodeEmergencyMove {
		return nodeEmergencyMove{
			lock: func(_ context.Context, lock bool) (string, error) {
				if lock {
					w.calls = append(w.calls, "lock")
					if lockKept != nil {
						return lockKept.Error(), nil
					}
					w.locked = true
				} else {
					w.calls = append(w.calls, "unlock")
					w.locked = false
				}
				return "", nil
			},
			move: func(context.Context) error {
				w.calls = append(w.calls, "move")
				if !w.locked {
					t.Fatal("moved without the start lock on the withdrawn version")
				}
				w.running = "22.12.0"
				if moveErr != nil {
					// The undo of a failed transition asks the old version to
					// start; the lock refuses it.
					if !w.locked {
						w.running = "22.11.0"
					}
					return moveErr
				}
				return nil
			},
			stop: func(context.Context) error {
				w.calls = append(w.calls, "stop")
				w.running = ""
				return stopErr
			},
			putBack: func(context.Context) error {
				w.calls = append(w.calls, "putback")
				return backErr
			},
		}
	}
	ctx := context.Background()

	w := &world{}
	if note, err := build(w, nil, nil, nil, nil).run(ctx, "22.11.0", "22.12.0"); err != nil || note != "" {
		t.Fatalf("move: %q %v", note, err)
	}
	if strings.Join(w.calls, ",") != "lock,move,unlock" || w.running != "22.12.0" || w.locked {
		t.Fatalf("move: %+v", w)
	}

	w = &world{}
	_, err := build(w, errors.New("no answer on port 3001"), nil, nil, nil).run(ctx, "22.11.0", "22.12.0")
	if stepErrorKind(err) != "verified_failure" || !strings.Contains(err.Error(), "stays on the maintenance page") {
		t.Fatalf("failed move: %v", err)
	}
	if strings.Join(w.calls, ",") != "lock,move,stop,putback" || w.running != "" || !w.locked {
		t.Fatalf("failed move left: %+v", w)
	}

	w = &world{}
	_, err = build(w, errors.New("no answer"), nil, nil, errors.New("record changed")).run(ctx, "22.11.0", "22.12.0")
	if stepErrorKind(err) != "unknown" || w.running != "" || !w.locked {
		t.Fatalf("failed put-back: %v %+v", err, w)
	}

	w = &world{}
	_, err = build(w, nil, errors.New("the start lock was changed on the server"), nil, nil).run(ctx, "22.11.0", "22.12.0")
	if stepErrorKind(err) != "prerequisite" || strings.Join(w.calls, ",") != "lock" {
		t.Fatalf("edited lock: %v %+v", err, w)
	}
}

// d: the customer and its reseller are told where the administrator moved
// their site; a site the customer moved itself is not in that notice.
func TestEmergencyAdminMoveNotifiesCustomerAndReseller(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "php", "8.5", "8.4").Emergency.ID
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	// The admin-managed customer's site moves itself first.
	f.exec(t, `UPDATE sites SET php_version = '8.4' WHERE id = 9941`)
	f.panel.noteEmergencySiteMoved(context.Background(), 9911)
	f.step(t, id, "move", confirmBody("PHP 8.5", `,"target_version":"8.4"`), http.StatusAccepted, "")
	f.run(t)
	read := func(user int) []securityNoticeView {
		rec := f.as(t, user, http.MethodGet, securityNoticesPath, "")
		wantStatus(t, rec, http.StatusOK, "")
		var got struct {
			Notices []securityNoticeView `json:"notices"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		moved := []securityNoticeView{}
		for _, notice := range got.Notices {
			if notice.Kind == "moved" {
				moved = append(moved, notice)
			}
		}
		return moved
	}
	customer := read(9703)
	if len(customer) != 1 || len(customer[0].Body.Sites) != 1 || customer[0].Body.Sites[0].Domain != "php85.matrix.test" ||
		customer[0].Body.Sites[0].MovedTo != "8.4" || customer[0].Audience != "customer" || len(customer[0].Actions) != 0 {
		t.Fatalf("customer moved notice: %+v", customer)
	}
	reseller := read(9702)
	if len(reseller) != 1 || reseller[0].Audience != "reseller" || reseller[0].Body.Sites[0].Customer != "matrix-customer" {
		t.Fatalf("reseller moved notice: %+v", reseller)
	}
	if moved := read(9704); len(moved) != 0 {
		t.Fatalf("a site its customer moved got an administrator's move notice: %+v", moved)
	}
	subject, body := securityNoticeMail("moved", "customer", customer[0].Body, "https://panel.example.test")
	if !strings.Contains(subject, "Your site was moved") || !strings.Contains(body, "php85.matrix.test -> PHP 8.4") || !strings.Contains(body, "Nothing was deleted") {
		t.Fatalf("moved mail: %s\n%s", subject, body)
	}
}

// b: a withdrawn version or engine is not installed.
func TestEmergencyWithdrawnInstallsAreRefused(t *testing.T) {
	withdrawn := map[string]map[string]bool{"php": {"8.5": true}, "node": {"22.11.0": true}, "mariadb": {"": true}}
	cases := []struct{ service, pkg, php, want string }{
		{"php-fpm", "php8.5-fpm", "", "PHP 8.5"},
		{"php-fpm", "php8.5-curl", "", "PHP 8.5"},
		{"php-fpm", "php8.4-fpm", "", ""},
		{"phpmyadmin", "", "8.5", "PHP 8.5"},
		{"node", "22.11.0", "", "Node.js 22.11.0"},
		{"node", "22.12.0", "", ""},
		{"mariadb", "", "", "MariaDB"},
		{"postgresql", "", "", ""},
	}
	for _, c := range cases {
		if got := withdrawnInstallName(withdrawn, c.service, c.pkg, c.php); got != c.want {
			t.Errorf("%s %s %s: %q, want %q", c.service, c.pkg, c.php, got, c.want)
		}
	}

	f := newEmergencyFixture(t)
	f.declare(t, "php", "8.5", "8.4")
	f.declare(t, "node", "22.11.0", "")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/services/install", f.panel.handleServiceInstall)
	mux.HandleFunc("/api/v1/runtimes/node", f.panel.handleNodeRuntimes)
	f.handler = f.panel.requireAuth(mux)
	rec := f.as(t, 9701, http.MethodPost, "/api/v1/services/install", `{"service_id":"php-fpm","package":"php8.5-fpm","request_id":"0123456789abcdef0123456789abcdef"}`)
	wantStatus(t, rec, http.StatusConflict, errCodeSecurityEmergencyWithdrawn)
	if !strings.Contains(rec.Body.String(), "Nothing was installed: PHP 8.5 is withdrawn") || !strings.Contains(rec.Body.String(), "re-enables it") {
		t.Fatalf("refusal: %s", rec.Body.String())
	}
	rec = f.as(t, 9701, http.MethodPost, "/api/v1/runtimes/node", `{"version":"22.11.0","request_id":"0123456789abcdef0123456789abcde0"}`)
	wantStatus(t, rec, http.StatusConflict, errCodeSecurityEmergencyWithdrawn)
	if f.count(t, `SELECT COUNT(*) FROM service_operations`) != 0 {
		t.Fatal("a refused install queued an operation")
	}
	// Inside an operation (a server setup step or a recovered one).
	if failure := f.panel.withdrawnInstallFailure(context.Background(), "node", "22.11.0", ""); failure == nil || failure.Code != "security_emergency_withdrawn" {
		t.Fatalf("operation refusal: %+v", failure)
	}
	if failure := f.panel.withdrawnInstallFailure(context.Background(), "php-fpm", "php8.4-fpm", ""); failure != nil {
		t.Fatalf("an install that is not withdrawn was refused: %+v", failure)
	}
}
