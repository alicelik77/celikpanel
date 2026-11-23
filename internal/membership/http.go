package membership

// Port of portal-membership/http.php (origin/main 2584a566e): the account pages and
// the panels' API at /account/?action=..., with the same headers, CSRF and origin
// checks, limits and answers. The panel contract:
//
//	POST ?action=activate  {"key","server_id","hostname"}                -> 200 envelope
//	POST ?action=refresh   {"license_id","server_id","activation_token"} -> 200 envelope
//	400 {"error":<code>}; 405 + Allow: POST; 413 above 8192 bytes; 503 {"error":"service_unavailable"}
//
// The bound address is the web server's peer address only (REMOTE_ADDR in CGI, the
// TCP peer for the loopback server); never a request field or forwarded header.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	unavailableText       = "Üyelik hizmetine şu anda erişilemiyor. Lütfen daha sonra tekrar deneyin.\nMembership is temporarily unavailable. Please try again later."
	maxBody               = 8192
	maxFormBody           = 8 << 20 // PHP's post_max_size default
)

// Handler serves /account/. One request opens the configuration and the database,
// as each PHP request does.
type Handler struct {
	// Private is the private directory (config.json, members.sqlite, sessions/).
	Private string
	// AllowTestKey is the throwaway public key a test named; nil in production,
	// where only the pinned production key is accepted.
	AllowTestKey ed25519.PublicKey
	// RemoteAddr is the web server's view of the client (REMOTE_ADDR). Required.
	RemoteAddr func(*http.Request) string
	// QueryString is $_SERVER['QUERY_STRING']; nil reads the request URL.
	QueryString func(*http.Request) string
	// ServiceClock and WallClock: nil means the real clock (tests fix them).
	ServiceClock func() int64
	WallClock    func() time.Time
	// Mailer builds the transport from the configuration; nil uses Mailer{} as
	// configured (tests add their certificate authority here).
	Mailer func(Config) Mailer
	// Log receives one line per failed request: a class, never a value.
	Log func(string)
}

func (h *Handler) wall() time.Time {
	if h.WallClock != nil {
		return h.WallClock()
	}
	return time.Now()
}

func (h *Handler) log(class string) {
	if h.Log != nil {
		h.Log("CelikPanel membership request failed: " + class)
	}
}

type exchange struct {
	h       *Handler
	w       http.ResponseWriter
	r       *http.Request
	api     bool
	sess    *session
	cookie  string
	secure  bool
	written bool
}

// ServeHTTP answers one request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("Content-Security-Policy", contentSecurityPolicy)
	query := r.URL.RawQuery
	if h.QueryString != nil {
		query = h.QueryString(r)
	}
	get := phpVars{}
	parsePHPQuery(query, get)
	action, arrayAction := "home", false
	if v, ok := get["action"]; ok {
		action, arrayAction = v.str, v.array
	}
	x := &exchange{h: h, w: w, r: r, api: !arrayAction && (action == "activate" || action == "refresh")}
	if err := x.run(get, action, arrayAction); err != nil {
		x.fail(err)
	}
}

func (x *exchange) fail(err error) {
	if x.written {
		return
	}
	status, code, class := http.StatusServiceUnavailable, "service_unavailable", "error"
	var problem Problem
	if errors.As(err, &problem) {
		status, code, class = http.StatusBadRequest, string(problem), "problem"
	}
	x.finishSession()
	if x.api {
		x.w.Header().Set("Content-Type", "application/json")
		x.w.WriteHeader(status)
		x.w.Write([]byte(`{"error":` + jsonString(code) + `}`))
	} else {
		x.w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		x.w.WriteHeader(status)
		x.w.Write([]byte(unavailableText))
	}
	x.written = true
	// Do not log messages containing request data, credentials or SQL values.
	x.h.log(class)
}

// respond writes a status with an empty body (PHP's default text/html type).
func (x *exchange) respond(status int) {
	if x.w.Header().Get("Content-Type") == "" {
		x.w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	}
	x.finishSession()
	x.w.WriteHeader(status)
	x.written = true
}

func (x *exchange) finishSession() {
	if x.sess == nil {
		return
	}
	if x.sess.cookieChanged() {
		http.SetCookie(x.w, &http.Cookie{Name: x.cookie, Value: x.sess.id, Path: "/", Secure: x.secure, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	if err := x.sess.save(); err != nil {
		x.h.log("session")
	}
	x.sess = nil
}

func (x *exchange) run(get phpVars, action string, arrayAction bool) error {
	config, err := LoadConfig(x.h.Private)
	if err != nil {
		return err
	}
	if err := CheckSigningKey(config.SigningKey, x.h.AllowTestKey); err != nil {
		return err
	}
	service, err := Open(filepath.Join(x.h.Private, "members.sqlite"), config.SigningKey, x.h.sender(config), x.h.ServiceClock)
	if err != nil {
		return err
	}
	defer service.Close()
	ip := x.h.RemoteAddr(x.r)
	if x.r.ContentLength > maxBody {
		x.respond(http.StatusRequestEntityTooLarge)
		return nil
	}
	if x.api {
		return x.panelAPI(service, config, ip, action)
	}
	return x.pages(service, config, ip, get, action, arrayAction)
}

// sender is http.php's $send closure.
func (h *Handler) sender(config Config) Sender {
	return func(email, purpose, token string) error {
		link := config.Origin + "/account/?action=" + purpose + "#token=" + token
		subject, intro := "CelikPanel password reset / Parola yenileme", "Reset your password / Parolanizi yenileyin:"
		if purpose == "verify" {
			subject, intro = "CelikPanel email verification / E-posta dogrulama", "Confirm your email / E-postanizi dogrulayin:"
		}
		body := "CelikPanel\n\n" + intro + "\n" + link +
			"\n\nValid for 30 minutes. If you did not request this, ignore this email.\n30 dakika gecerlidir. Istegi siz yapmadiysaniz bu e-postayi yok sayin.\n"
		// Test delivery is available only in the explicit loopback development config.
		if config.TestMailDir != "" && strings.HasPrefix(config.Origin, "http://127.0.0.1:") {
			slash := func(s string) string { return strings.ReplaceAll(jsonString(s), "/", `\/`) }
			record := `{"email":` + slash(email) + `,"purpose":` + slash(purpose) + `,"token":` + slash(token) + `}`
			_ = os.WriteFile(filepath.Join(config.TestMailDir, randomHex(12)+".json"), []byte(record), 0o600)
			return nil
		}
		if config.SMTPPresent && config.SMTP == nil {
			return ErrMailUnavailable // a present but unusable smtp block never falls back
		}
		mailer := Mailer{Sender: config.Sender, SMTP: config.SMTP}
		if h.Mailer != nil {
			mailer = h.Mailer(config)
		}
		return mailer.Send(email, subject, body)
	}
}

// panelAPI is http.php:35-53.
func (x *exchange) panelAPI(service *Service, config Config, ip, action string) error {
	x.w.Header().Set("Content-Type", "application/json")
	if x.r.Method != http.MethodPost {
		x.w.Header().Set("Allow", "POST")
		x.w.WriteHeader(http.StatusMethodNotAllowed)
		x.w.Write([]byte(`{"error":"method_not_allowed"}`))
		x.written = true
		return nil
	}
	if err := service.Limit("api:"+ip, 60, 60); err != nil {
		return err
	}
	contentType, _, _ := strings.Cut(x.r.Header.Get("Content-Type"), ";")
	if phpLower(phpTrim(contentType)) != "application/json" {
		return ErrInvalidRequest
	}
	raw, err := io.ReadAll(io.LimitReader(x.r.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(raw) > maxBody {
		return ErrInvalidRequest
	}
	input, ok := decodePanelRequest(raw)
	if !ok {
		return ErrInvalidRequest
	}
	allowed := map[string]bool{"key": true, "server_id": true, "hostname": true}
	if action == "refresh" {
		allowed = map[string]bool{"license_id": true, "server_id": true, "activation_token": true}
	}
	for name := range input {
		if !allowed[name] {
			return ErrInvalidRequest
		}
	}
	// REMOTE_ADDR must be provided by the web server, never a request header.
	observed := ip
	if config.TestMailDir != "" && strings.HasPrefix(config.Origin, "http://127.0.0.1:") && ip == "127.0.0.1" {
		observed = "8.8.8.8"
	}
	var envelope Envelope
	if action == "activate" {
		envelope, err = service.Activate(input["key"], input["server_id"], input["hostname"], observed)
	} else {
		envelope, err = service.Refresh(input["license_id"], input["server_id"], input["activation_token"], observed)
	}
	if err != nil {
		return err
	}
	x.w.WriteHeader(http.StatusOK)
	x.w.Write(envelope.JSON())
	x.written = true
	return nil
}

// decodePanelRequest is json_decode($raw, true, 8) followed by is_array and "every
// value is a string". A JSON list is an array to PHP too: an empty one passes with
// no fields, a non-empty one has the keys 0, 1, ... which are never allowed.
func decodePanelRequest(raw []byte) (map[string]string, bool) {
	if !utf8.Valid(raw) || loneSurrogate(raw) {
		return nil, false // PHP: malformed UTF-8 / unpaired UTF-16 surrogate
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	switch v := value.(type) {
	case map[string]any:
		out := map[string]string{}
		for name, field := range v {
			s, ok := field.(string)
			if !ok {
				return nil, false
			}
			out[name] = s
		}
		return out, true
	case []any:
		if len(v) == 0 {
			return map[string]string{}, true
		}
		return nil, false
	}
	return nil, false
}

// loneSurrogate finds a \uD800-\uDFFF escape that is not a proper pair; PHP refuses
// it where Go would quietly put U+FFFD.
func loneSurrogate(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !inString {
			if c == '"' {
				inString = true
			}
			continue
		}
		switch c {
		case '"':
			inString = false
		case '\\':
			if i+1 < len(raw) && raw[i+1] == 'u' && i+5 < len(raw) {
				u := hexValue(raw[i+2 : i+6])
				switch {
				case u >= 0xD800 && u <= 0xDBFF:
					if i+11 < len(raw) && raw[i+6] == '\\' && raw[i+7] == 'u' {
						if low := hexValue(raw[i+8 : i+12]); low >= 0xDC00 && low <= 0xDFFF {
							i += 11
							continue
						}
					}
					return true
				case u >= 0xDC00 && u <= 0xDFFF:
					return true
				}
				i += 5
				continue
			}
			i++
		}
	}
	return false
}

func hexValue(b []byte) int {
	v := 0
	for _, c := range b {
		if !isHex(c) {
			return -1
		}
		v = v<<4 | int(unhex(c))
	}
	return v
}

// readPost fills $_POST: urlencoded and multipart bodies only, up to PHP's default
// post_max_size (a larger body leaves $_POST empty, as PHP does).
func readPost(r *http.Request) phpVars {
	post := phpVars{}
	media, params, _ := parsePHPContentType(r.Header.Get("Content-Type"))
	switch media {
	case "application/x-www-form-urlencoded":
		raw, err := io.ReadAll(io.LimitReader(r.Body, maxFormBody+1))
		if err != nil || len(raw) > maxFormBody {
			return post
		}
		parsePHPQuery(string(raw), post)
	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" {
			return post
		}
		reader := multipart.NewReader(io.LimitReader(r.Body, maxFormBody+1), boundary)
		for {
			part, err := reader.NextPart()
			if err != nil {
				return post
			}
			name := part.FormName()
			if name == "" || part.FileName() != "" {
				part.Close()
				continue
			}
			value, err := io.ReadAll(io.LimitReader(part, maxFormBody))
			part.Close()
			if err != nil {
				return post
			}
			if i := strings.IndexByte(name, 0); i >= 0 {
				name = name[:i]
			}
			name = strings.TrimLeft(name, " ")
			if name != "" {
				post.set(name, string(value))
			}
		}
	}
	return post
}

// parsePHPContentType is SAPI.c's reading of the POST type: lower case, cut at the
// first ";", "," or " ".
func parsePHPContentType(value string) (string, map[string]string, error) {
	media := value
	if i := strings.IndexAny(value, ";, "); i >= 0 {
		media = value[:i]
	}
	_, params, err := mime.ParseMediaType(value)
	return phpLower(media), params, err
}

// pages is http.php:55-102 and view.php.
func (x *exchange) pages(service *Service, config Config, ip string, get phpVars, action string, arrayAction bool) error {
	if err := service.Limit("page:"+ip, 120, 60); err != nil {
		return err
	}
	x.secure = strings.HasPrefix(config.Origin, "https://")
	x.cookie = "celikpanel_member_dev"
	if x.secure {
		x.cookie = "__Host-celikpanel_member"
	}
	incoming := ""
	if c, err := x.r.Cookie(x.cookie); err == nil {
		incoming = c.Value
	}
	now := x.h.wall()
	x.sess = startSession(filepath.Join(x.h.Private, "sessions"), incoming, x.h.wall)
	// session_start() with the default "nocache" limiter.
	x.w.Header().Set("Expires", "Thu, 19 Nov 1981 08:52:00 GMT")
	x.w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	x.w.Header().Set("Pragma", "no-cache")
	data := &x.sess.data
	if data.Created == nil || *data.Created < now.Unix()-sessionLifetime {
		*data = sessionData{}
		x.sess.regenerate()
		created := now.Unix()
		data.Created = &created
	}
	if data.CSRF == "" {
		data.CSRF = randomHex(32)
	}
	lang := "tr"
	if v, ok := get["lang"]; ok {
		if !v.array && v.str == "en" {
			lang = "en"
		}
	} else if data.Lang == "en" {
		lang = "en"
	}
	data.Lang = lang
	var memberID, epoch int64
	if data.Member != nil {
		memberID = *data.Member
	}
	if data.Epoch != nil {
		epoch = *data.Epoch
	}
	member, err := service.MemberByEpoch(memberID, epoch)
	if err != nil {
		return err
	}
	if member == nil {
		data.Member, data.Epoch = nil, nil
	}
	errorCode, notice, issued := "", data.Notice, data.Issued
	data.Notice, data.Issued = "", nil
	post := phpVars{}
	switch x.r.Method {
	case http.MethodPost:
		post = readPost(x.r)
		target, err := x.form(service, config, ip, post, action, arrayAction, lang, member, now)
		if err == nil {
			x.w.Header().Set("Location", "/account/?action="+target+"&lang="+lang)
			x.respond(http.StatusSeeOther)
			return nil
		}
		var problem Problem
		if !errors.As(err, &problem) {
			return err
		}
		errorCode = string(problem)
	case http.MethodGet:
	default:
		x.respond(http.StatusMethodNotAllowed)
		return nil
	}
	if arrayAction {
		// view.php:41 isset($titles[array]) throws a TypeError: PHP answers 503.
		return errors.New("array action")
	}
	if action == "home" && member == nil {
		action = "login"
	}
	var licenses []License
	if member != nil {
		if licenses, err = service.Licenses(member.ID); err != nil {
			return err
		}
	}
	view := &page{lang: lang, action: action, member: member, err: errorCode, notice: notice, issued: issued,
		licenses: licenses, now: service.Now(), csrf: data.CSRF, get: get, post: post}
	body := view.render()
	x.w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	x.finishSession()
	x.w.WriteHeader(http.StatusOK)
	io.WriteString(x.w, body)
	x.written = true
	return nil
}

// form is the POST switch of http.php:66-94; it answers the action to redirect to.
func (x *exchange) form(service *Service, config Config, ip string, post phpVars, action string, arrayAction bool, lang string, member *Member, now time.Time) (string, error) {
	if values, ok := x.r.Header["Origin"]; ok && strings.Join(values, ", ") != config.Origin {
		return "", ErrInvalidRequest
	}
	csrf, ok := post["csrf"]
	if !ok || csrf.array || !hmac.Equal([]byte(x.sess.data.CSRF), []byte(csrf.str)) {
		return "", ErrSessionChanged
	}
	for _, v := range post {
		if v.array {
			return "", ErrInvalidRequest
		}
	}
	if err := service.Limit("form:"+ip, 30, 900); err != nil {
		return "", err
	}
	if arrayAction {
		return "", ErrInvalidRequest
	}
	email, password, id := post.str("email"), post.str("password"), post.str("id")
	data := &x.sess.data
	switch action {
	case "register":
		if post.str("terms") != "yes" {
			return "", ErrAcceptTerms
		}
		if err := service.Register(email); err != nil {
			return "", err
		}
		data.Notice = "mail_sent"
	case "resend", "forgot":
		purpose := "verify"
		if action == "forgot" {
			purpose = "reset"
		}
		if err := service.RequestToken(email, purpose); err != nil {
			return "", err
		}
		data.Notice = "mail_sent"
	case "verify", "reset":
		if err := service.ConsumeToken(post.str("token"), action, password); err != nil {
			return "", err
		}
		data.Notice = "email_verified"
		if action == "reset" {
			data.Notice = "password_reset"
		}
	case "login":
		m, err := service.Login(email, password)
		if err != nil {
			return "", err
		}
		x.sess.regenerate()
		created := now.Unix()
		*data = sessionData{Member: &m.ID, Epoch: &m.Epoch, Lang: lang, Created: &created, CSRF: randomHex(32)}
		return "home", nil
	case "logout":
		*data = sessionData{}
		x.sess.regenerate()
	case "issue", "release", "renew", "reveal", "rotate", "remove":
		if member == nil {
			return "", ErrAuthenticationRequired
		}
		switch action {
		case "issue":
			issued, err := service.Issue(member.ID)
			if err != nil {
				return "", err
			}
			data.Issued = &issued
		case "release":
			if err := service.Release(member.ID, id, password); err != nil {
				return "", err
			}
			data.Notice = "server_released"
		case "reveal":
			issued, err := service.Reveal(member.ID, id, password)
			if err != nil {
				return "", err
			}
			data.Issued = &issued
		case "remove":
			if err := service.Remove(member.ID, id, password); err != nil {
				return "", err
			}
			data.Notice = "license_removed"
		case "rotate":
			issued, err := service.Rotate(member.ID, id, password)
			if err != nil {
				return "", err
			}
			data.Issued = &issued
		default:
			if err := service.Renew(member.ID, id, password); err != nil {
				return "", err
			}
			data.Notice = "renewed"
		}
		return "home", nil
	default:
		return "", ErrInvalidRequest
	}
	return "login", nil
}

// PeerAddress is the loopback server's REMOTE_ADDR: the TCP peer, never a header.
func PeerAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
