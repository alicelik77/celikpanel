package remotebackup

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// AWS Signature Version 4 against the published examples: the test suite's
// get-vanilla and the four S3 examples of "Signature Calculations for the
// Authorization Header" (examplebucket, 24 May 2013).
func TestS3SigV4KnownVectors(t *testing.T) {
	s3Time := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	const s3Key, s3Secret = "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	cases := []struct {
		name string
		in   sigV4Input
		want string
	}{
		{
			name: "get-vanilla",
			in: sigV4Input{Method: "GET", Path: "/", Headers: map[string]string{
				"host": "example.amazonaws.com", "x-amz-date": "20150830T123600Z"},
				PayloadHash: emptySHA256, AccessKey: "AKIDEXAMPLE", Secret: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
				Region: "us-east-1", Service: "service", Time: time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)},
			want: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31",
		},
		{
			name: "s3 get object",
			in: sigV4Input{Method: "GET", Path: "/test.txt", Headers: map[string]string{
				"host": "examplebucket.s3.amazonaws.com", "range": "bytes=0-9",
				"x-amz-content-sha256": emptySHA256, "x-amz-date": "20130524T000000Z"},
				PayloadHash: emptySHA256, AccessKey: s3Key, Secret: s3Secret, Region: "us-east-1", Service: "s3", Time: s3Time},
			want: "Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41",
		},
		{
			name: "s3 put object",
			in: sigV4Input{Method: "PUT", Path: "/" + uriEncode("test$file.text", false), Headers: map[string]string{
				"date": "Fri, 24 May 2013 00:00:00 GMT", "host": "examplebucket.s3.amazonaws.com",
				"x-amz-content-sha256": "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072",
				"x-amz-date":           "20130524T000000Z", "x-amz-storage-class": "REDUCED_REDUNDANCY"},
				PayloadHash: "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072",
				AccessKey:   s3Key, Secret: s3Secret, Region: "us-east-1", Service: "s3", Time: s3Time},
			want: "Signature=98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd",
		},
		{
			name: "s3 get bucket lifecycle",
			in: sigV4Input{Method: "GET", Path: "/", Query: url.Values{"lifecycle": {""}}, Headers: map[string]string{
				"host": "examplebucket.s3.amazonaws.com", "x-amz-content-sha256": emptySHA256, "x-amz-date": "20130524T000000Z"},
				PayloadHash: emptySHA256, AccessKey: s3Key, Secret: s3Secret, Region: "us-east-1", Service: "s3", Time: s3Time},
			want: "Signature=fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543",
		},
		{
			name: "s3 list objects",
			in: sigV4Input{Method: "GET", Path: "/", Query: url.Values{"max-keys": {"2"}, "prefix": {"J"}}, Headers: map[string]string{
				"host": "examplebucket.s3.amazonaws.com", "x-amz-content-sha256": emptySHA256, "x-amz-date": "20130524T000000Z"},
				PayloadHash: emptySHA256, AccessKey: s3Key, Secret: s3Secret, Region: "us-east-1", Service: "s3", Time: s3Time},
			want: "Signature=34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7",
		},
	}
	for _, tc := range cases {
		got := signV4(tc.in)
		if !strings.HasSuffix(got, tc.want) && got != tc.want {
			t.Errorf("%s:\n got %s\nwant …%s", tc.name, got, tc.want)
		}
	}
	if sha := sha256.Sum256([]byte("Welcome to Amazon S3.")); hex.EncodeToString(sha[:]) != "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072" {
		t.Fatal("payload hash of the PUT example")
	}
}

// fakeS3 is a small in-memory S3: it checks the payload SHA-256 and the
// Content-MD5 the way S3 does, keeps multipart uploads, and can fail requests.
type fakeS3 struct {
	mu        sync.Mutex
	objects   map[string][]byte
	uploads   map[string]map[int][]byte
	aborted   int
	completed int
	requests  []string
	// failPart: part number -> how many times to answer 500 before accepting
	failPart map[int]int
	// corruptPart: part number -> how many times the body is damaged in transit
	corruptPart map[int]int
	authFail    bool
	sawAuth     []string
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: map[string][]byte{}, uploads: map[string]map[int][]byte{}, failPart: map[int]int{}, corruptPart: map[int]int{}}
}

func s3XMLError(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0"?><Error><Code>%s</Code><Message>m</Message></Error>`, code)
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RawQuery)
	f.sawAuth = append(f.sawAuth, r.Header.Get("Authorization"))
	if f.authFail {
		s3XMLError(w, 403, "SignatureDoesNotMatch")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIDTEST/") {
		s3XMLError(w, 403, "AccessDenied")
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if parts[0] != "bucket" {
		s3XMLError(w, 404, "NoSuchBucket")
		return
	}
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}
	body, _ := io.ReadAll(r.Body)
	q := r.URL.Query()
	partNumber, _ := strconv.Atoi(q.Get("partNumber"))
	if partNumber > 0 && f.corruptPart[partNumber] > 0 {
		f.corruptPart[partNumber]--
		body = append([]byte(nil), body...)
		body[0] ^= 0xff
	}
	sum := sha256.Sum256(body)
	if r.Header.Get("x-amz-content-sha256") != hex.EncodeToString(sum[:]) {
		s3XMLError(w, 400, "XAmzContentSHA256Mismatch")
		return
	}
	if m := r.Header.Get("Content-MD5"); m != "" {
		got := md5.Sum(body)
		if m != base64.StdEncoding.EncodeToString(got[:]) {
			s3XMLError(w, 400, "BadDigest")
			return
		}
	}
	switch {
	case r.Method == http.MethodGet && key == "":
		prefix := q.Get("prefix")
		keys := []string{}
		for k := range f.objects {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		fmt.Fprint(w, `<ListBucketResult>`)
		for _, k := range keys {
			fmt.Fprintf(w, `<Contents><Key>%s</Key><Size>%d</Size><LastModified>2026-11-03T10:00:00.000Z</LastModified></Contents>`, k, len(f.objects[k]))
		}
		fmt.Fprint(w, `<IsTruncated>false</IsTruncated></ListBucketResult>`)
	case r.Method == http.MethodPost && q.Has("uploads"):
		id := fmt.Sprintf("up%d", len(f.uploads)+1)
		f.uploads[id] = map[int][]byte{}
		fmt.Fprintf(w, `<InitiateMultipartUploadResult><UploadId>%s</UploadId></InitiateMultipartUploadResult>`, id)
	case r.Method == http.MethodPut && partNumber > 0:
		if f.failPart[partNumber] > 0 {
			f.failPart[partNumber]--
			s3XMLError(w, 500, "InternalError")
			return
		}
		up, ok := f.uploads[q.Get("uploadId")]
		if !ok {
			s3XMLError(w, 404, "NoSuchUpload")
			return
		}
		up[partNumber] = body
		m := md5.Sum(body)
		w.Header().Set("ETag", `"`+hex.EncodeToString(m[:])+`"`)
	case r.Method == http.MethodPost && q.Has("uploadId"):
		up, ok := f.uploads[q.Get("uploadId")]
		if !ok {
			s3XMLError(w, 404, "NoSuchUpload")
			return
		}
		var req completeUpload
		if err := xml.Unmarshal(body, &req); err != nil {
			s3XMLError(w, 400, "MalformedXML")
			return
		}
		var all, md5s []byte
		for i, p := range req.Parts {
			if p.PartNumber != i+1 {
				s3XMLError(w, 400, "InvalidPartOrder")
				return
			}
			all = append(all, up[p.PartNumber]...)
			m := md5.Sum(up[p.PartNumber])
			md5s = append(md5s, m[:]...)
		}
		f.objects[key] = all
		delete(f.uploads, q.Get("uploadId"))
		f.completed++
		c := md5.Sum(md5s)
		fmt.Fprintf(w, `<CompleteMultipartUploadResult><ETag>"%s-%d"</ETag></CompleteMultipartUploadResult>`, hex.EncodeToString(c[:]), len(req.Parts))
	case r.Method == http.MethodDelete && q.Has("uploadId"):
		delete(f.uploads, q.Get("uploadId"))
		f.aborted++
		w.WriteHeader(204)
	case r.Method == http.MethodPut:
		f.objects[key] = body
		m := md5.Sum(body)
		w.Header().Set("ETag", `"`+hex.EncodeToString(m[:])+`"`)
	case r.Method == http.MethodHead || r.Method == http.MethodGet:
		data, ok := f.objects[key]
		if !ok {
			if r.Method == http.MethodHead {
				w.WriteHeader(404)
				return
			}
			s3XMLError(w, 404, "NoSuchKey")
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Last-Modified", "Tue, 03 Nov 2026 10:00:00 GMT")
		if strings.Contains(key, "multi") {
			w.Header().Set("ETag", `"`+"00000000000000000000000000000000-0"+`"`) // differs: HEAD keeps the completion's
			w.Header().Del("ETag")
		} else {
			m := md5.Sum(data)
			w.Header().Set("ETag", `"`+hex.EncodeToString(m[:])+`"`)
		}
		if r.Method == http.MethodGet {
			w.Write(data)
		}
	case r.Method == http.MethodDelete:
		if _, ok := f.objects[key]; !ok {
			s3XMLError(w, 404, "NoSuchKey")
			return
		}
		delete(f.objects, key)
		w.WriteHeader(204)
	default:
		s3XMLError(w, 400, "Unsupported")
	}
}

func testS3(t *testing.T, f *fakeS3, partSize int64) Store {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	store, err := NewS3(S3Config{Endpoint: srv.URL, Region: "eu-central-1", Bucket: "bucket", Prefix: "/backups/",
		AccessKeyID: "AKIDTEST", SecretAccessKey: "very-secret-value-123", PartSize: partSize,
		RetryDelay: time.Millisecond, allowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestS3RoundTripSinglePut(t *testing.T) {
	f := newFakeS3()
	store := testS3(t, f, 1<<20)
	ctx := context.Background()
	payload := []byte("small encrypted backup")
	res, err := store.Put(ctx, "srv/sites/a.example-d1/manual/x.gpg", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if res.Size != int64(len(payload)) || res.SHA256 != hex.EncodeToString(sum[:]) || res.Verified != "size+md5" {
		t.Fatalf("result %+v", res)
	}
	if _, ok := f.objects["backups/srv/sites/a.example-d1/manual/x.gpg"]; !ok {
		t.Fatal("object not stored under the prefix")
	}
	list, err := store.List(ctx, "srv/sites/")
	if err != nil || len(list) != 1 || list[0].Key != "srv/sites/a.example-d1/manual/x.gpg" || list[0].Size != int64(len(payload)) {
		t.Fatalf("list %+v %v", list, err)
	}
	rc, err := store.Get(ctx, list[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, payload) {
		t.Fatal("get returned other bytes")
	}
	if err := store.Delete(ctx, list[0].Key); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, list[0].Key); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := store.Stat(ctx, list[0].Key); KindOf(err) != KindNotFound {
		t.Fatalf("stat after delete: %v", err)
	}
}

// A multipart upload whose second part fails twice with 500 and whose third
// part arrives damaged once is still completed: the parts are sent again from
// memory and the result is checked against the parts' MD5.
func TestS3MultipartRetriesFailedAndDamagedParts(t *testing.T) {
	f := newFakeS3()
	f.failPart[2] = 2
	f.corruptPart[3] = 1
	store := testS3(t, f, 64<<10)
	payload := make([]byte, 64<<10*3+1234)
	rand.Read(payload)
	res, err := store.Put(context.Background(), "multi/big.gpg", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if res.SHA256 != hex.EncodeToString(sum[:]) || res.Size != int64(len(payload)) || res.Verified != "size+md5" {
		t.Fatalf("result %+v", res)
	}
	if !bytes.Equal(f.objects["backups/multi/big.gpg"], payload) {
		t.Fatal("stored bytes differ")
	}
	if f.completed != 1 || f.aborted != 0 {
		t.Fatalf("completed %d aborted %d", f.completed, f.aborted)
	}
}

// A part that keeps failing aborts the upload: no object, no open upload.
func TestS3MultipartAbortsOnPermanentFailure(t *testing.T) {
	f := newFakeS3()
	f.failPart[2] = 100
	store := testS3(t, f, 64<<10)
	payload := make([]byte, 64<<10*3)
	_, err := store.Put(context.Background(), "multi/big.gpg", bytes.NewReader(payload))
	if KindOf(err) != KindProvider || !Transient(err) {
		t.Fatalf("err %v kind %s", err, KindOf(err))
	}
	if len(f.objects) != 0 || len(f.uploads) != 0 || f.aborted != 1 {
		t.Fatalf("objects %d uploads %d aborted %d", len(f.objects), len(f.uploads), f.aborted)
	}
}

// Refused credentials are classified and never echoed.
func TestS3CredentialRefusalIsRedacted(t *testing.T) {
	f := newFakeS3()
	f.authFail = true
	store := testS3(t, f, 1<<20)
	_, err := store.Put(context.Background(), "a/b.gpg", strings.NewReader("x"))
	if KindOf(err) != KindCredentials {
		t.Fatalf("kind %s: %v", KindOf(err), err)
	}
	if strings.Contains(err.Error(), "very-secret-value-123") {
		t.Fatal("secret in error")
	}
	for _, auth := range f.sawAuth {
		if strings.Contains(auth, "very-secret-value-123") {
			t.Fatal("secret sent in a header")
		}
	}
	if len(f.requests) != 1 {
		t.Fatalf("credential refusal was retried: %d requests", len(f.requests))
	}
}

func TestS3ConfigValidation(t *testing.T) {
	good := S3Config{Endpoint: "https://s3.example.com", Bucket: "my-bucket", AccessKeyID: "AKID1234", SecretAccessKey: "secret-1234"}
	if _, err := NewS3(good); err != nil {
		t.Fatalf("good config: %v", err)
	}
	bad := []func(*S3Config){
		func(c *S3Config) { c.Endpoint = "http://s3.example.com" },
		func(c *S3Config) { c.Endpoint = "https://s3.example.com/path" },
		func(c *S3Config) { c.Endpoint = "https://user:pw@s3.example.com" },
		func(c *S3Config) { c.Bucket = "a" },
		func(c *S3Config) { c.Bucket = "bad/bucket" },
		func(c *S3Config) { c.Region = "EU Central" },
		func(c *S3Config) { c.Prefix = "a/../b" },
		func(c *S3Config) { c.AccessKeyID = "" },
		func(c *S3Config) { c.SecretAccessKey = "has space x" },
	}
	for i, mutate := range bad {
		cfg := good
		mutate(&cfg)
		if _, err := NewS3(cfg); KindOf(err) != KindConfig {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestS3StatusClassification(t *testing.T) {
	cases := []struct {
		status int
		code   string
		want   Kind
	}{
		{403, "AccessDenied", KindPermission},
		{403, "InvalidAccessKeyId", KindCredentials},
		{403, "RequestTimeTooSkewed", KindClock},
		{404, "NoSuchBucket", KindNotFound},
		{400, "AuthorizationHeaderMalformed", KindConfig},
		{503, "SlowDown", KindProvider},
		{400, "BadDigest", KindIntegrity},
		{403, "QuotaExceeded", KindQuota},
	}
	for _, tc := range cases {
		if got := KindOf(s3StatusError("put", tc.status, tc.code, "")); got != tc.want {
			t.Errorf("%d %s: %s, want %s", tc.status, tc.code, got, tc.want)
		}
	}
}
