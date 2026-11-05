package main

import (
	"compress/gzip"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Compression (v0.1.0-alpha.125, "faster panel" P3). At 170 ms RTT the cold
// load moved 375 KB of uncompressed boot assets and every lazy screen chunk
// went over the wire raw; the bundle budget already assumed gzip. Static
// assets are compressed once, at build time (web/scripts/precompress.mjs),
// and served as stored; JSON reads above 1 KiB are compressed per response
// with the standard library. Brotli would need a third-party module, so it is
// not used.
//
// Secrets: a compressed response that carries a secret next to text an
// attacker can choose leaks the secret through its length (BREACH). Only
// GET/HEAD reads are compressed, and never a read on a path that may carry a
// secret or a one-time value (passwords, credentials, keys, tokens, recovery,
// join codes, licences, account and VPN data, raw configuration files) or a
// machine-to-machine path. When unsure, a path is left uncompressed.
//
// Sıkıştırma: durağan dosyalar derlemede bir kez sıkıştırılır ve öyle sunulur;
// 1 KiB üzeri JSON okumaları standart kütüphaneyle sıkıştırılır. Gizli bilgi
// taşıyabilecek hiçbir yanıt sıkıştırılmaz (BREACH).

const jsonCompressMinBytes = 1024

// acceptsGzip reads Accept-Encoding: gzip (or *) with a non-zero quality,
// unless gzip itself is refused with q=0.
func acceptsGzip(r *http.Request) bool {
	accepted := false
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "gzip" && name != "x-gzip" && name != "*" {
			continue
		}
		quality := 1.0
		for _, param := range strings.Split(params, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
				if q, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
					quality = q
				}
			}
		}
		if quality <= 0 {
			if name != "*" {
				return false
			}
			continue
		}
		accepted = true
	}
	return accepted
}

// servePrecompressed serves the build-time .gz sibling of a static file to a
// client that accepts gzip. It reports whether it answered; the caller serves
// the file itself otherwise. The sibling must be a regular file (never a
// link), as the installed web tree is.
func servePrecompressed(w http.ResponseWriter, r *http.Request, filePath string) bool {
	if strings.HasSuffix(filePath, ".gz") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}
	info, err := os.Lstat(filePath + ".gz")
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	w.Header().Add("Vary", "Accept-Encoding")
	if !acceptsGzip(r) {
		return false
	}
	file, err := os.Open(filePath + ".gz")
	if err != nil {
		return false
	}
	defer file.Close()
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		return false
	}
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Content-Encoding", "gzip")
	http.ServeContent(w, r, "", info.ModTime(), file)
	return true
}

// jsonCompressionAllowed decides from the request alone whether a JSON answer
// may be compressed (the response decides the rest: type and size).
func jsonCompressionAllowed(r *http.Request) bool {
	path := r.URL.Path
	if r.Method != http.MethodGet && r.Method != http.MethodHead && !(r.Method == http.MethodPost && jsonCompressionPostReads[path]) {
		return false
	}
	if !strings.HasPrefix(path, "/api/") || remoteDNSMachinePath(path) || serverSetupLinkMachinePath(path) {
		return false
	}
	lower := strings.ToLower(path)
	for _, word := range jsonCompressionDeniedWords {
		if strings.Contains(lower, word) {
			return false
		}
	}
	return true
}

// jsonCompressionPostReads are POST requests that only read and answer no
// secret: the Components scan returns the same component list as its GET.
var jsonCompressionPostReads = map[string]bool{
	"/api/v1/managed-services/scan": true,
}

// jsonCompressionDeniedWords: any path containing one of these is never
// compressed (BREACH). Broad on purpose.
var jsonCompressionDeniedWords = []string{
	"/auth", "password", "passwordless", "secret", "token", "credential", "key",
	"totp", "2fa", "otp", "recovery", "join", "enrol", "invit", "licen",
	"account", "vpn", "/config", "backup", "export", "download", "database-users",
	"system-databases", "dbtool", "impersonat", "session", "certificate",
	"dkim", "transfer", "cpmove", "import",
}

var gzipWriterPool = sync.Pool{New: func() any {
	writer, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return writer
}}

// compressJSONResponses compresses JSON reads above jsonCompressMinBytes for
// a client that accepts gzip. Everything else passes through unchanged.
func compressJSONResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !jsonCompressionAllowed(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		writer := &gzipJSONWriter{ResponseWriter: w, head: r.Method == http.MethodHead}
		defer writer.finish()
		next.ServeHTTP(writer, r)
	})
}

// gzipJSONWriter holds the first jsonCompressMinBytes of a response, then
// decides once: a JSON answer of at least that size, not already encoded, is
// compressed; anything else is written as it came.
type gzipJSONWriter struct {
	http.ResponseWriter
	head    bool
	status  int
	buffer  []byte
	decided bool
	gz      *gzip.Writer
}

func (w *gzipJSONWriter) WriteHeader(status int) {
	if w.decided || w.status != 0 {
		if w.decided {
			w.ResponseWriter.WriteHeader(status)
		}
		return
	}
	// An informational answer is passed on at once and is not the status.
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
}

func (w *gzipJSONWriter) Write(data []byte) (int, error) {
	if !w.decided {
		w.buffer = append(w.buffer, data...)
		if len(w.buffer) < jsonCompressMinBytes {
			return len(data), nil
		}
		if err := w.decide(); err != nil {
			return 0, err
		}
		return len(data), nil
	}
	if w.gz != nil {
		return w.gz.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *gzipJSONWriter) decide() error {
	w.decided = true
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	header := w.ResponseWriter.Header()
	mediaType, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	compress := !w.head && len(w.buffer) >= jsonCompressMinBytes &&
		status != http.StatusNoContent && status != http.StatusNotModified &&
		header.Get("Content-Encoding") == "" && mediaType == "application/json"
	buffered := w.buffer
	w.buffer = nil
	if !compress {
		w.ResponseWriter.WriteHeader(status)
		if len(buffered) == 0 {
			return nil
		}
		_, err := w.ResponseWriter.Write(buffered)
		return err
	}
	header.Del("Content-Length")
	header.Set("Content-Encoding", "gzip")
	w.ResponseWriter.WriteHeader(status)
	w.gz = gzipWriterPool.Get().(*gzip.Writer)
	w.gz.Reset(w.ResponseWriter)
	_, err := w.gz.Write(buffered)
	return err
}

// finish writes what is still held and closes the compressed stream.
func (w *gzipJSONWriter) finish() {
	if !w.decided {
		if w.status == 0 && len(w.buffer) == 0 {
			return // nothing was written: net/http answers 200 with no body
		}
		_ = w.decide()
	}
	if w.gz != nil {
		_ = w.gz.Close()
		gzipWriterPool.Put(w.gz)
		w.gz = nil
	}
}
