package main

import (
	"bytes"
	"compress/gzip"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Compression (v0.1.0-alpha.125, "faster panel" P3). At 170 ms RTT the cold
// load moved 375 KB of uncompressed boot assets and every lazy screen chunk
// went over the wire raw; the bundle budget already assumed gzip. Static
// assets are compressed once per file in memory (below) and served from
// there; JSON reads above 1 KiB are compressed per response
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
// Sıkıştırma: durağan dosyalar ilk istekte bir kez sıkıştırılıp bellekte tutulur;
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

// Static assets are compressed by the Panel itself, once per file, the first
// time a client that accepts gzip asks for it, and kept in memory for the
// process lifetime (keyed by path, size and modification time, so a replaced
// file is compressed again). The release therefore carries no .gz copies: a
// build-time copy made by Node's zlib can differ between CPUs (its deflate
// takes SIMD paths), which would make the release archive depend on the
// build host. Go's compress/gzip writes the same bytes on every host, and
// nothing compressed is written to disk at all.
//
// Durağan dosyalar ilk istekte Panel tarafından bir kez sıkıştırılır ve
// bellekte tutulur; sürüm arşivinde .gz kopyası yoktur, bu yüzden arşiv
// derleme makinesine bağlı değildir.

const (
	staticCompressMinBytes = 1024
	staticCompressMaxBytes = 16 << 20
)

// staticCompressible are the text assets worth compressing; images and fonts
// are already compressed.
var staticCompressible = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".html": true, ".svg": true,
	".json": true, ".txt": true, ".webmanifest": true,
}

type compressedAsset struct {
	size    int64
	modTime time.Time
	data    []byte // nil: compressing saves too little, serve the file
}

var compressedAssets sync.Map // file path -> *compressedAsset

func compressedAssetFor(filePath string, info os.FileInfo) *compressedAsset {
	if cached, ok := compressedAssets.Load(filePath); ok {
		asset := cached.(*compressedAsset)
		if asset.size == info.Size() && asset.modTime.Equal(info.ModTime()) {
			return asset
		}
	}
	source, err := os.ReadFile(filePath)
	if err != nil || int64(len(source)) != info.Size() {
		return nil
	}
	asset := &compressedAsset{size: info.Size(), modTime: info.ModTime()}
	var packed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&packed, gzip.BestCompression)
	if err != nil {
		return nil
	}
	if _, err := writer.Write(source); err != nil || writer.Close() != nil {
		return nil
	}
	// A copy that saves less than a tenth is not worth it.
	if packed.Len()*10 <= len(source)*9 {
		asset.data = packed.Bytes()
	}
	compressedAssets.Store(filePath, asset)
	return asset
}

// servePrecompressed serves a gzip copy of a static text file to a client
// that accepts gzip. It reports whether it answered; the caller serves the
// file itself otherwise. Only a regular file (never a link) is compressed,
// as the installed web tree holds.
func servePrecompressed(w http.ResponseWriter, r *http.Request, filePath string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if !staticCompressible[ext] {
		return false
	}
	info, err := os.Lstat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() < staticCompressMinBytes || info.Size() > staticCompressMaxBytes {
		return false
	}
	w.Header().Add("Vary", "Accept-Encoding")
	if !acceptsGzip(r) {
		return false
	}
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		return false
	}
	asset := compressedAssetFor(filePath, info)
	if asset == nil || asset.data == nil {
		return false
	}
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Content-Encoding", "gzip")
	http.ServeContent(w, r, "", asset.modTime, bytes.NewReader(asset.data))
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
