package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebmailReadinessRequiresDirectHTTP200(t *testing.T) {
	redirectReached := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectReached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	tests := []struct {
		name   string
		status int
		want   bool
	}{
		{name: "ready", status: http.StatusOK, want: true},
		{name: "other success is not the login page", status: http.StatusNoContent},
		{name: "server failure", status: http.StatusBadGateway},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte("bounded body"))
			}))
			defer server.Close()
			client := &http.Client{Timeout: time.Second}
			if got := webmailEndpointReady(context.Background(), client, server.URL); got != test.want {
				t.Fatalf("ready = %v, want %v", got, test.want)
			}
		})
	}

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer redirector.Close()
	client := &http.Client{
		Timeout: time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if webmailEndpointReady(context.Background(), client, redirector.URL) {
		t.Fatal("redirect was accepted as ready")
	}
	if redirectReached {
		t.Fatal("readiness followed an external redirect")
	}
}

func TestWebmailReadinessTimeoutFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := &http.Client{Timeout: 20 * time.Millisecond}
	if webmailEndpointReady(context.Background(), client, server.URL) {
		t.Fatal("timed-out webmail endpoint was reported ready")
	}
}

func TestWebmailEntryRateLimitReturnsRetryHint(t *testing.T) {
	previous := webmailRequestLimiter
	webmailRequestLimiter = newRateLimiter(0, time.Minute)
	t.Cleanup(func() { webmailRequestLimiter = previous })

	request := httptest.NewRequest(http.MethodGet, "/webmail/", nil)
	request.RemoteAddr = "192.0.2.45:1234"
	recorder := httptest.NewRecorder()

	(&Panel{}).handleWebmailEntry(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", recorder.Code)
	}
	if got := recorder.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After = %q, want 60", got)
	}
}
