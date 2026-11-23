package membership

// Review of PR #361, finding 4: a request still in flight must not bring back a
// session that a sign-out ended meanwhile (PHP's session lock). Also session expiry.

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type signedIn struct {
	a      *apiServer
	client *http.Client
	cookie *http.Cookie
	csrf   string
}

var csrfPattern = regexp.MustCompile(`name="csrf" value="([a-f0-9]{64})"`)

func (s *signedIn) page(t *testing.T, method, query string, form url.Values) (int, string) {
	t.Helper()
	var response *http.Response
	var err error
	if method == http.MethodPost {
		response, err = s.client.PostForm(s.a.url+"?"+query, form)
	} else {
		response, err = s.client.Get(s.a.url + "?" + query)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if m := csrfPattern.FindStringSubmatch(string(raw)); m != nil {
		s.csrf = m[1]
	}
	return response.StatusCode, string(raw)
}

func signIn(t *testing.T) *signedIn {
	t.Helper()
	a := newAPIServer(t)
	a.issueKey()
	jar, _ := cookiejar.New(nil)
	s := &signedIn{a: a, client: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
	s.page(t, http.MethodGet, "action=login", nil)
	if _, body := s.page(t, http.MethodPost, "action=login", url.Values{"csrf": {s.csrf}, "email": {"api@example.com"},
		"password": {"api-test-password-123"}}); !strings.Contains(body, "Lisanslarınız") {
		t.Fatal("sign-in failed")
	}
	base, _ := url.Parse(a.url)
	for _, c := range jar.Cookies(base) {
		if c.Name == "celikpanel_member_dev" {
			s.cookie = c
		}
	}
	if s.cookie == nil {
		t.Fatal("no session cookie")
	}
	return s
}

// withCookie sends one request with exactly this session cookie (no jar).
func withCookie(t *testing.T, target string, cookie *http.Cookie, method string, form url.Values) string {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	request, _ := http.NewRequest(method, target, body)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	request.AddCookie(cookie)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Error(err)
		return ""
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return string(raw)
}

func TestSignOutIsNotUndoneByARequestInFlight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the session lock is flock; the service runs on Linux")
	}
	s := signIn(t)
	handler := s.a.handler
	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	handler.beforeSave = func() {
		fired := false
		once.Do(func() { fired = true })
		if fired {
			close(held)
			<-release
		}
	}
	csrf := s.csrf
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // request A: signed in, still running
		defer wg.Done()
		withCookie(t, s.a.url+"?action=home", s.cookie, http.MethodGet, nil)
	}()
	<-held
	signedOut := make(chan struct{})
	go func() { // request B: the member signs out with the same session
		withCookie(t, s.a.url+"?action=logout", s.cookie, http.MethodPost, url.Values{"csrf": {csrf}})
		close(signedOut)
	}()
	select {
	case <-signedOut:
		close(release) // let request A finish so the server can close, then fail
		wg.Wait()
		t.Fatal("the sign-out did not wait for the request in flight")
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	<-signedOut
	handler.beforeSave = nil
	if body := withCookie(t, s.a.url+"?action=home", s.cookie, http.MethodGet, nil); !strings.Contains(body, "Hesabınıza giriş yapın") {
		t.Fatal("the old session is signed in again after the sign-out")
	}
}

func TestExpiredSessionIsNotUsed(t *testing.T) {
	s := signIn(t)
	path := filepath.Join(s.a.dir, "sessions", "go-sess_"+s.cookie.Value)
	old := time.Now().Add(-13 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if body := withCookie(t, s.a.url+"?action=home", s.cookie, http.MethodGet, nil); !strings.Contains(body, "Hesabınıza giriş yapın") {
		t.Fatal("a session older than 12 hours was used")
	}
}
