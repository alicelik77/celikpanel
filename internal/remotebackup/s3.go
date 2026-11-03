package remotebackup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// S3-compatible object storage: AWS S3, Backblaze B2 (S3 API), Wasabi,
// MinIO, Cloudflare R2. Requests are signed with AWS Signature Version 4 and
// carry the SHA-256 of their body (x-amz-content-sha256) and its MD5
// (Content-MD5), so the storage itself refuses a body damaged on the way.
// Large objects go up in parts (multipart upload); a failed part is sent
// again from memory, and a failed upload is aborted so no partial object is
// left. After an upload the object's size is read back, and its ETag is
// compared with the MD5 of the parts where the storage reports one.
//
// S3 uyumlu depolama. İstekler SigV4 ile imzalanır, gövdenin SHA-256 ve MD5
// özeti taşınır; büyük nesneler parça parça gönderilir, başarısız parça
// bellekten yeniden gönderilir, yarım yükleme iptal edilir.

// S3Config is one S3-compatible destination.
type S3Config struct {
	Endpoint        string // https://s3.eu-central-1.amazonaws.com, https://<account>.r2.cloudflarestorage.com...
	Region          string // "us-east-1" when empty; R2 uses "auto"
	Bucket          string
	Prefix          string // folder inside the bucket; may be empty
	AccessKeyID     string
	SecretAccessKey string
	// VirtualHosted addresses the bucket as <bucket>.<endpoint host> instead
	// of <endpoint>/<bucket> (path style, the default every listed provider
	// accepts).
	VirtualHosted bool

	HTTPClient *http.Client  // nil: a client with sane timeouts
	PartSize   int64         // 0: 16 MiB
	Attempts   int           // per request; 0: 4
	RetryDelay time.Duration // first retry delay; doubles; 0: 2s

	allowHTTP bool             // tests only
	now       func() time.Time // tests only
}

const (
	s3DefaultPartSize = 16 << 20
	s3MinPartSize     = 5 << 20
	s3MaxPartSize     = 256 << 20
	s3MaxParts        = 9000 // below the protocol's 10000, leaving room for a size estimate error
	s3MaxListed       = 100000
	emptySHA256       = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

var (
	s3RegionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	s3BucketPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{1,61}[a-zA-Z0-9]$`)
	s3KeyIDPattern  = regexp.MustCompile(`^[A-Za-z0-9+/=_.-]{3,256}$`)
)

type s3Store struct {
	cfg      S3Config
	endpoint *url.URL
	client   *http.Client
	prefix   string
	mu       sync.Mutex
}

// NewS3 validates cfg and returns the store. It makes no network request.
func NewS3(cfg S3Config) (Store, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	endpoint, _ := url.Parse(cfg.Endpoint)
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 0, // bodies are large; requests are bounded by ctx and the transport timeouts
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   20 * time.Second,
				ResponseHeaderTimeout: 5 * time.Minute,
				ExpectContinueTimeout: 2 * time.Second,
				MaxIdleConnsPerHost:   2,
				IdleConnTimeout:       60 * time.Second,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &s3Store{cfg: cfg, endpoint: endpoint, client: client, prefix: strings.Trim(cfg.Prefix, "/")}, nil
}

func (c *S3Config) normalize() error {
	c.Endpoint = strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return newError(KindConfig, "settings", "the endpoint must be an address like https://s3.example.com, without a path", nil)
	}
	if u.Scheme != "https" && !(c.allowHTTP && u.Scheme == "http") {
		return newError(KindConfig, "settings", "the endpoint must use https://", nil)
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	if !s3RegionPattern.MatchString(c.Region) {
		return newError(KindConfig, "settings", "the region may hold only lowercase letters, digits and dashes", nil)
	}
	if !s3BucketPattern.MatchString(c.Bucket) || strings.Contains(c.Bucket, "..") {
		return newError(KindConfig, "settings", "the bucket name is not valid", nil)
	}
	c.Prefix = strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if !validPrefix(c.Prefix) {
		return newError(KindConfig, "settings", "the folder (prefix) is not valid", nil)
	}
	if !s3KeyIDPattern.MatchString(c.AccessKeyID) {
		return newError(KindConfig, "settings", "the access key ID is not valid", nil)
	}
	if len(c.SecretAccessKey) < 8 || len(c.SecretAccessKey) > 512 || strings.ContainsAny(c.SecretAccessKey, " \r\n\t") {
		return newError(KindConfig, "settings", "the secret access key is not valid", nil)
	}
	if c.PartSize == 0 {
		c.PartSize = s3DefaultPartSize
	}
	if c.PartSize < s3MinPartSize && !c.allowHTTP {
		return newError(KindConfig, "settings", "the part size is too small", nil)
	}
	if c.PartSize > s3MaxPartSize {
		c.PartSize = s3MaxPartSize
	}
	if c.Attempts <= 0 {
		c.Attempts = 4
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = 2 * time.Second
	}
	if c.now == nil {
		c.now = time.Now
	}
	return nil
}

// SizeHinter may be implemented by a reader given to Put: the expected
// number of bytes, used to pick a part size big enough for the part limit.
type SizeHinter interface{ SizeHint() int64 }

func (s *s3Store) full(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

// ----- Signature Version 4 -----

type sigV4Input struct {
	Method      string
	Path        string // canonical URI, already encoded
	Query       url.Values
	Headers     map[string]string // lowercase name -> value; must include host and x-amz-date
	PayloadHash string
	AccessKey   string
	Secret      string
	Region      string
	Service     string
	Time        time.Time
}

// signV4 returns the Authorization header value.
func signV4(in sigV4Input) string {
	names := make([]string, 0, len(in.Headers))
	for name := range in.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(trimHeaderValue(in.Headers[name]))
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaders := strings.Join(names, ";")
	canonicalRequest := strings.Join([]string{
		in.Method, in.Path, canonicalQuery(in.Query), canonicalHeaders.String(), signedHeaders, in.PayloadHash,
	}, "\n")
	date := in.Time.UTC().Format("20060102")
	scope := date + "/" + in.Region + "/" + in.Service + "/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + in.Time.UTC().Format("20060102T150405Z") + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	key := hmacSHA256([]byte("AWS4"+in.Secret), date)
	key = hmacSHA256(key, in.Region)
	key = hmacSHA256(key, in.Service)
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, stringToSign))
	return "AWS4-HMAC-SHA256 Credential=" + in.AccessKey + "/" + scope + ", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func trimHeaderValue(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

func canonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	type pair struct{ k, v string }
	pairs := []pair{}
	for k, vs := range q {
		for _, v := range vs {
			pairs = append(pairs, pair{uriEncode(k, true), uriEncode(v, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

// uriEncode is the encoding SigV4 specifies: unreserved characters stay,
// everything else is %XX (uppercase); '/' stays unless encodeSlash.
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{c})))
		}
	}
	return b.String()
}

// ----- requests -----

type s3Request struct {
	method  string
	key     string // full key ("" = the bucket)
	query   url.Values
	headers map[string]string
	body    []byte // nil = empty
}

func (s *s3Store) url(key string, query url.Values) (*url.URL, string) {
	u := *s.endpoint
	path := "/"
	if s.cfg.VirtualHosted {
		u.Host = s.cfg.Bucket + "." + s.endpoint.Host
		path = "/" + uriEncode(key, false)
	} else {
		path = "/" + s.cfg.Bucket
		if key != "" {
			path += "/" + uriEncode(key, false)
		}
	}
	u.Path = ""
	u.RawPath = ""
	u.Opaque = ""
	if q := canonicalQuery(query); q != "" {
		u.RawQuery = q
	}
	return &u, path
}

func (s *s3Store) newHTTPRequest(ctx context.Context, r s3Request) (*http.Request, error) {
	u, path := s.url(r.key, r.query)
	full := u.Scheme + "://" + u.Host + path
	if u.RawQuery != "" {
		full += "?" + u.RawQuery
	}
	var body io.Reader
	payload := emptySHA256
	if r.body != nil {
		body = bytes.NewReader(r.body)
		sum := sha256.Sum256(r.body)
		payload = hex.EncodeToString(sum[:])
	}
	req, err := http.NewRequestWithContext(ctx, r.method, full, body)
	if err != nil {
		return nil, newError(KindConfig, "request", "the request could not be built", nil)
	}
	if r.body != nil {
		req.ContentLength = int64(len(r.body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(r.body)), nil }
	}
	now := s.cfg.now().UTC()
	headers := map[string]string{
		"host":                 u.Host,
		"x-amz-date":           now.Format("20060102T150405Z"),
		"x-amz-content-sha256": payload,
	}
	for k, v := range r.headers {
		headers[strings.ToLower(k)] = v
	}
	for k, v := range headers {
		if k == "host" {
			continue
		}
		req.Header.Set(k, v)
	}
	req.Host = u.Host
	req.Header.Set("Authorization", signV4(sigV4Input{
		Method: r.method, Path: path, Query: r.query, Headers: headers, PayloadHash: payload,
		AccessKey: s.cfg.AccessKeyID, Secret: s.cfg.SecretAccessKey, Region: s.cfg.Region,
		Service: "s3", Time: now,
	}))
	return req, nil
}

// do sends a request with retries for transient failures and returns a
// response with a 2xx status (body open) or a classified error.
func (s *s3Store) do(ctx context.Context, op string, r s3Request, retryIntegrity bool) (*http.Response, error) {
	var last error
	delay := s.cfg.RetryDelay
	for attempt := 1; attempt <= s.cfg.Attempts; attempt++ {
		req, err := s.newHTTPRequest(ctx, r)
		if err != nil {
			return nil, err
		}
		resp, err := s.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		if err != nil {
			last = classifyTransportError(op, ctx, err)
		} else {
			last = s3ResponseError(op, resp)
		}
		retry := Transient(last) || (retryIntegrity && KindOf(last) == KindIntegrity)
		if !retry || attempt == s.cfg.Attempts || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, classifyTransportError(op, ctx, ctx.Err())
		case <-time.After(delay):
		}
		delay *= 2
	}
	return nil, last
}

func classifyTransportError(op string, ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return newError(KindNetwork, op, "the request was stopped before the storage answered (time limit or cancel)", nil)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return newError(KindNetwork, op, "the storage did not answer in time", nil)
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return newError(KindNetwork, op, "the storage's address could not be resolved: "+safeText(dnsErr.Name), nil)
	}
	if strings.Contains(err.Error(), "certificate") || strings.Contains(err.Error(), "x509") {
		return newError(KindConfig, op, "the storage's TLS certificate is not trusted by this server", nil)
	}
	return newError(KindNetwork, op, "the connection to the storage failed", nil)
}

type s3ErrorBody struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

func s3ResponseError(op string, resp *http.Response) error {
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	var body s3ErrorBody
	_ = xml.Unmarshal(raw, &body)
	return s3StatusError(op, resp.StatusCode, body.Code, body.Message)
}

func s3StatusError(op string, status int, code, message string) error {
	detail := strconv.Itoa(status)
	if code != "" {
		detail += " " + safeText(code)
	}
	lowerMsg := strings.ToLower(message)
	switch {
	case code == "RequestTimeTooSkewed":
		return newError(KindClock, op, "the storage refused this server's clock ("+detail+"); correct the server time", nil)
	case code == "InvalidAccessKeyId" || code == "SignatureDoesNotMatch" || code == "InvalidToken" ||
		code == "ExpiredToken" || code == "InvalidSecurity" || code == "InvalidAccessKey":
		return newError(KindCredentials, op, "the storage refused the access key or secret ("+detail+")", nil)
	case code == "AuthorizationHeaderMalformed" || code == "PermanentRedirect" || code == "IllegalLocationConstraintException" ||
		status == http.StatusMovedPermanently || status == http.StatusTemporaryRedirect:
		return newError(KindConfig, op, "the bucket is not at this endpoint or region ("+detail+")", nil)
	case code == "QuotaExceeded" || code == "StorageQuotaExceeded" || code == "EntityTooLarge" ||
		status == http.StatusInsufficientStorage || strings.Contains(lowerMsg, "cap exceeded") ||
		strings.Contains(lowerMsg, "quota"):
		return newError(KindQuota, op, "the storage is full or its limit is reached ("+detail+")", nil)
	case code == "BadDigest" || code == "InvalidDigest" || code == "XAmzContentSHA256Mismatch" || code == "IncompleteBody":
		return newError(KindIntegrity, op, "the storage received damaged data ("+detail+")", nil)
	case code == "NoSuchBucket":
		return newError(KindNotFound, op, "the bucket does not exist ("+detail+")", nil)
	case code == "NoSuchKey" || status == http.StatusNotFound:
		return newError(KindNotFound, op, "the object does not exist ("+detail+")", nil)
	case status == http.StatusUnauthorized:
		return newError(KindCredentials, op, "the storage refused the access key or secret ("+detail+")", nil)
	case status == http.StatusForbidden:
		return newError(KindPermission, op, "the key is not allowed to do this in the bucket ("+detail+")", nil)
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || code == "SlowDown" ||
		code == "RequestTimeout" || code == "InternalError" || status >= 500:
		return newError(KindProvider, op, "the storage had a temporary problem ("+detail+")", nil)
	default:
		return newError(KindProvider, op, "the storage refused the request ("+detail+")", nil)
	}
}

// safeText keeps a short, printable, single-line excerpt.
func safeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 120 {
			break
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ----- Store -----

func (s *s3Store) Put(ctx context.Context, key string, r io.Reader) (PutResult, error) {
	if !validKey(key) {
		return PutResult{}, newError(KindConfig, "put", "invalid object key", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	full := s.full(key)
	partSize := s.cfg.PartSize
	if h, ok := r.(SizeHinter); ok && h.SizeHint() > 0 {
		for h.SizeHint()/partSize >= s3MaxParts && partSize < s3MaxPartSize {
			partSize *= 2
		}
		if partSize > s3MaxPartSize {
			partSize = s3MaxPartSize
		}
	}
	whole := sha256.New()
	buf := make([]byte, partSize)
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return PutResult{}, sourceReadError(err)
	}
	if err != nil { // the whole object fits in one request
		data := buf[:n]
		whole.Write(data)
		sum := md5.Sum(data)
		resp, err := s.do(ctx, "put", s3Request{method: http.MethodPut, key: full, body: data,
			headers: map[string]string{"content-md5": base64.StdEncoding.EncodeToString(sum[:]), "content-type": "application/octet-stream"}}, true)
		if err != nil {
			return PutResult{}, err
		}
		etag := resp.Header.Get("ETag")
		resp.Body.Close()
		verified, err := s.verifyUploaded(ctx, full, int64(n), etag, hex.EncodeToString(sum[:]))
		if err != nil {
			return PutResult{}, err
		}
		return PutResult{Size: int64(n), SHA256: hex.EncodeToString(whole.Sum(nil)), Verified: verified}, nil
	}
	return s.putMultipart(ctx, full, r, buf, whole)
}

func sourceReadError(err error) error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return newError(KindIntegrity, "put", "the backup could not be read on this server while sending", nil)
}

type completedPart struct {
	XMLName    xml.Name `xml:"Part"`
	PartNumber int      `xml:"PartNumber"`
	ETag       string   `xml:"ETag"`
}

type completeUpload struct {
	XMLName xml.Name        `xml:"CompleteMultipartUpload"`
	Parts   []completedPart `xml:"Part"`
}

func (s *s3Store) putMultipart(ctx context.Context, full string, r io.Reader, buf []byte, whole io.Writer) (result PutResult, returnErr error) {
	resp, err := s.do(ctx, "put", s3Request{method: http.MethodPost, key: full, query: url.Values{"uploads": {""}},
		headers: map[string]string{"content-type": "application/octet-stream"}}, false)
	if err != nil {
		return PutResult{}, err
	}
	var created struct {
		UploadID string `xml:"UploadId"`
	}
	err = xml.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&created)
	resp.Body.Close()
	if err != nil || created.UploadID == "" {
		return PutResult{}, newError(KindProvider, "put", "the storage did not start the upload", nil)
	}
	uploadID := created.UploadID
	defer func() {
		if returnErr != nil {
			abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if resp, err := s.do(abortCtx, "abort", s3Request{method: http.MethodDelete, key: full,
				query: url.Values{"uploadId": {uploadID}}}, false); err == nil {
				resp.Body.Close()
			}
		}
	}()
	var parts []completedPart
	var md5s []byte
	var total int64
	shaWriter := whole.(interface{ Sum([]byte) []byte })
	n := len(buf)
	last := false
	for number := 1; ; number++ {
		if number > 10000 {
			return PutResult{}, newError(KindConfig, "put", "the backup is too large for this storage's part limit", nil)
		}
		data := buf[:n]
		whole.Write(data)
		sum := md5.Sum(data)
		resp, err := s.do(ctx, "put", s3Request{method: http.MethodPut, key: full,
			query:   url.Values{"partNumber": {strconv.Itoa(number)}, "uploadId": {uploadID}},
			headers: map[string]string{"content-md5": base64.StdEncoding.EncodeToString(sum[:])},
			body:    data}, true)
		if err != nil {
			return PutResult{}, err
		}
		etag := resp.Header.Get("ETag")
		resp.Body.Close()
		if bare := strings.Trim(etag, `"`); isMD5Hex(bare) && !strings.EqualFold(bare, hex.EncodeToString(sum[:])) {
			return PutResult{}, newError(KindIntegrity, "put", "the storage's checksum of a part does not match what was sent", nil)
		}
		if etag == "" {
			return PutResult{}, newError(KindProvider, "put", "the storage did not confirm a part", nil)
		}
		parts = append(parts, completedPart{PartNumber: number, ETag: etag})
		md5s = append(md5s, sum[:]...)
		total += int64(len(data))
		if last {
			break
		}
		if ctx.Err() != nil {
			return PutResult{}, classifyTransportError("put", ctx, ctx.Err())
		}
		var readErr error
		n, readErr = io.ReadFull(r, buf)
		if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return PutResult{}, sourceReadError(readErr)
		}
		if n == 0 {
			break
		}
		last = readErr != nil
	}
	body, _ := xml.Marshal(completeUpload{Parts: parts})
	resp, err = s.do(ctx, "put", s3Request{method: http.MethodPost, key: full,
		query: url.Values{"uploadId": {uploadID}}, body: body,
		headers: map[string]string{"content-type": "application/xml"}}, false)
	if err != nil {
		return PutResult{}, err
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	// A completion can fail after a 200 status: the error is in the body.
	var completeErr s3ErrorBody
	if xml.Unmarshal(raw, &completeErr) == nil && completeErr.Code != "" && bytes.Contains(raw, []byte("<Error")) {
		return PutResult{}, s3StatusError("put", http.StatusOK, completeErr.Code, completeErr.Message)
	}
	var completed struct {
		ETag string `xml:"ETag"`
	}
	_ = xml.Unmarshal(raw, &completed)
	composite := md5.Sum(md5s)
	expected := hex.EncodeToString(composite[:]) + "-" + strconv.Itoa(len(parts))
	verified, err := s.verifyUploaded(ctx, full, total, completed.ETag, expected)
	if err != nil {
		return PutResult{}, err
	}
	return PutResult{Size: total, SHA256: hex.EncodeToString(shaWriter.Sum(nil)), Verified: verified}, nil
}

func isMD5Hex(s string) bool {
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// verifyUploaded reads the object's size back and compares its ETag with the
// expected MD5 form when the storage uses that form.
func (s *s3Store) verifyUploaded(ctx context.Context, full string, size int64, etag, expected string) (string, error) {
	obj, headETag, err := s.head(ctx, full)
	if err != nil {
		return "", err
	}
	if obj.Size != size {
		return "", newError(KindIntegrity, "verify", "the stored size does not match what was sent", nil)
	}
	if headETag != "" {
		etag = headETag
	}
	bare := strings.ToLower(strings.Trim(etag, `"`))
	looksMD5 := isMD5Hex(bare) || (strings.Contains(bare, "-") && isMD5Hex(strings.SplitN(bare, "-", 2)[0]))
	if looksMD5 && strings.Contains(expected, "-") == strings.Contains(bare, "-") {
		if bare != strings.ToLower(expected) {
			return "", newError(KindIntegrity, "verify", "the storage's checksum does not match what was sent", nil)
		}
		return "size+md5", nil
	}
	return "size", nil
}

func (s *s3Store) head(ctx context.Context, full string) (Object, string, error) {
	resp, err := s.do(ctx, "stat", s3Request{method: http.MethodHead, key: full}, false)
	if err != nil {
		return Object{}, "", err
	}
	resp.Body.Close()
	obj := Object{Size: resp.ContentLength}
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		obj.Modified = t.UTC()
	}
	return obj, resp.Header.Get("ETag"), nil
}

func (s *s3Store) Stat(ctx context.Context, key string) (Object, error) {
	if !validKey(key) {
		return Object{}, newError(KindConfig, "stat", "invalid object key", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	obj, _, err := s.head(ctx, s.full(key))
	obj.Key = key
	return obj, err
}

func (s *s3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if !validKey(key) {
		return nil, newError(KindConfig, "get", "invalid object key", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resp, err := s.do(ctx, "get", s3Request{method: http.MethodGet, key: s.full(key)}, false)
	if err != nil {
		return nil, err
	}
	return &s3Body{rc: resp.Body}, nil
}

type s3Body struct{ rc io.ReadCloser }

func (b *s3Body) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, newError(KindNetwork, "get", "the download from the storage broke off", nil)
	}
	return n, err
}

func (b *s3Body) Close() error { return b.rc.Close() }

type listResult struct {
	Contents []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

func (s *s3Store) List(ctx context.Context, prefix string) ([]Object, error) {
	if !validPrefix(prefix) {
		return nil, newError(KindConfig, "list", "invalid prefix", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fullPrefix := prefix
	if s.prefix != "" {
		fullPrefix = s.prefix + "/" + prefix
	}
	out := []Object{}
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "max-keys": {"1000"}}
		if fullPrefix != "" {
			q.Set("prefix", fullPrefix)
		}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := s.do(ctx, "list", s3Request{method: http.MethodGet, query: q}, false)
		if err != nil {
			return nil, err
		}
		var page listResult
		err = xml.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, newError(KindProvider, "list", "the storage's listing could not be read", nil)
		}
		for _, c := range page.Contents {
			key := c.Key
			if s.prefix != "" {
				if !strings.HasPrefix(key, s.prefix+"/") {
					continue
				}
				key = strings.TrimPrefix(key, s.prefix+"/")
			}
			if !validKey(key) {
				continue
			}
			obj := Object{Key: key, Size: c.Size}
			if t, err := time.Parse(time.RFC3339, c.LastModified); err == nil {
				obj.Modified = t.UTC()
			}
			out = append(out, obj)
			if len(out) > s3MaxListed {
				return nil, newError(KindProvider, "list", "too many objects", nil)
			}
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			break
		}
		token = page.NextContinuationToken
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	if !validKey(key) {
		return newError(KindConfig, "delete", "invalid object key", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	resp, err := s.do(ctx, "delete", s3Request{method: http.MethodDelete, key: s.full(key)}, false)
	if err != nil {
		if KindOf(err) == KindNotFound && !strings.Contains(err.Error(), "bucket") {
			return nil
		}
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *s3Store) Close() error {
	s.client.CloseIdleConnections()
	return nil
}
