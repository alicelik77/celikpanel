// Command membership is the Go member service (internal/membership), a port of
// portal-membership/ at parity. Entry points:
//
//	membership cgi          one request as a CGI program (also chosen automatically
//	                        when GATEWAY_INTERFACE is set; the command line is then
//	                        ignored, because a web server may pass the query string
//	                        of an "isindex" request as arguments)
//	membership serve [-listen 127.0.0.1:8379]
//	                        a loopback HTTP server: /account/ only, the TCP peer as
//	                        REMOTE_ADDR (tests; a proxy in front would need a
//	                        trusted-proxy rule that does not exist yet)
//	membership check-db     read-only counts of the live database: can every live
//	                        licence be served with the configured key?
//	membership public-key   prints license_public_key=<hex> like init.php
//
// The private directory is $CELIKPANEL_MEMBERSHIP_PRIVATE (an absolute path), always
// named explicitly; the program never guesses it from where it is installed. The
// service opens members.sqlite there only if it already exists with its licences
// table; otherwise every request is answered 503 and nothing is created.
//
// The service refuses to start with a signing key whose public half is not the
// pinned production key (internal/licensing.PublicKeyHex). Only a test names a
// throwaway key: -test-signing-public-key <hex> (serve, check-db, public-key) or
// CELIKPANEL_MEMBERSHIP_TEST_SIGNING_PUBLIC_KEY (cgi, set by the web server's own
// configuration). It never creates or writes a key.
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/cgi"
	"os"
	"path/filepath"
	"time"

	"github.com/alicelik/celikpanel/internal/membership"
)

const testKeyVariable = "CELIKPANEL_MEMBERSHIP_TEST_SIGNING_PUBLIC_KEY"

func main() {
	restrictUmask()
	if _, ok := os.LookupEnv("GATEWAY_INTERFACE"); ok {
		os.Exit(runCGI())
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "cgi":
		os.Exit(runCGI())
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "check-db":
		os.Exit(runCheckDB(os.Args[2:]))
	case "public-key":
		os.Exit(runPublicKey(os.Args[2:]))
	}
	usage()
	os.Exit(2)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: membership cgi | serve [-listen 127.0.0.1:8379] [-test-signing-public-key HEX] | check-db [-test-signing-public-key HEX] | public-key [-test-signing-public-key HEX]")
}

// privateDir is named explicitly, never derived from where the program sits: a guess
// that pointed at the wrong folder would find no database (503) at best.
func privateDir() (string, error) {
	dir := os.Getenv("CELIKPANEL_MEMBERSHIP_PRIVATE")
	if dir == "" || !filepath.IsAbs(dir) {
		return "", errors.New("set CELIKPANEL_MEMBERSHIP_PRIVATE to the absolute path of the private directory")
	}
	return dir, nil
}

func parseTestKey(value string) (ed25519.PublicKey, error) {
	if value == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(value)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("the test signing public key must be 64 hex characters")
	}
	if hex.EncodeToString(key) == hex.EncodeToString(membership.ProductionPublicKey()) {
		return nil, errors.New("the test signing public key names the production key")
	}
	return key, nil
}

// loadChecked reads the configuration and refuses an unpinned key.
func loadChecked(testKey ed25519.PublicKey) (string, membership.Config, error) {
	dir, err := privateDir()
	if err != nil {
		return "", membership.Config{}, err
	}
	config, err := membership.LoadConfig(dir)
	if err != nil {
		return "", membership.Config{}, err
	}
	if err := membership.CheckSigningKey(config.SigningKey, testKey); err != nil {
		return "", membership.Config{}, err
	}
	return dir, config, nil
}

func logLine(line string) { fmt.Fprintln(os.Stderr, line) }

func runCGI() int {
	testKey, err := parseTestKey(os.Getenv(testKeyVariable))
	if err != nil {
		logLine("CelikPanel membership: " + err.Error())
		return 1
	}
	dir, err := privateDir()
	if err != nil {
		// The handler answers 503 for an empty directory: panels keep their receipts.
		logLine("CelikPanel membership: CELIKPANEL_MEMBERSHIP_PRIVATE is not set to an absolute path")
		dir = ""
	}
	handler := &membership.Handler{
		Private:      dir,
		AllowTestKey: testKey,
		RemoteAddr: func(*http.Request) string {
			if address, ok := os.LookupEnv("REMOTE_ADDR"); ok {
				return address
			}
			return "unknown"
		},
		QueryString: func(r *http.Request) string {
			if query, ok := os.LookupEnv("QUERY_STRING"); ok {
				return query
			}
			return r.URL.RawQuery
		},
		Log: logLine,
	}
	if err := cgi.Serve(handler); err != nil {
		logLine("CelikPanel membership: not a CGI request")
		return 1
	}
	return 0
}

func runServe(args []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8379", "loopback address to listen on")
	testHex := flags.String("test-signing-public-key", "", "accept this throwaway key instead of the production key (tests)")
	if flags.Parse(args) != nil {
		return 2
	}
	testKey, err := parseTestKey(*testHex)
	if err != nil {
		logLine("membership serve: " + err.Error())
		return 2
	}
	host, _, err := net.SplitHostPort(*listen)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		logLine("membership serve: listen only on a loopback address such as 127.0.0.1:8379")
		return 2
	}
	dir, config, err := loadChecked(testKey)
	if err != nil {
		logLine("membership serve: " + err.Error())
		return 1
	}
	probe, err := membership.OpenExisting(filepath.Join(dir, "members.sqlite"), config.SigningKey, nil, nil)
	if err != nil {
		logLine("membership serve: " + err.Error() + " (nothing was created)")
		return 1
	}
	probe.Close()
	handler := &membership.Handler{Private: dir, AllowTestKey: testKey, RemoteAddr: membership.PeerAddress, Log: logLine}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/" && r.URL.Path != "/account/index.php" {
			http.NotFound(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, MaxHeaderBytes: 64 << 10}
	logLine("membership serve: listening on " + *listen)
	if err := server.ListenAndServe(); err != nil {
		logLine("membership serve: " + err.Error())
		return 1
	}
	return 0
}

func runCheckDB(args []string) int {
	flags := flag.NewFlagSet("check-db", flag.ContinueOnError)
	testHex := flags.String("test-signing-public-key", "", "accept this throwaway key instead of the production key (tests)")
	if flags.Parse(args) != nil {
		return 2
	}
	testKey, err := parseTestKey(*testHex)
	if err != nil {
		logLine("membership check-db: " + err.Error())
		return 2
	}
	dir, config, err := loadChecked(testKey)
	if err != nil {
		logLine("membership check-db: " + err.Error())
		return 1
	}
	report, err := membership.CheckDB(filepath.Join(dir, "members.sqlite"), config.SigningKey, time.Now().Unix())
	if err != nil {
		logLine("membership check-db: " + err.Error())
		return 1
	}
	report.Print(os.Stdout)
	if !report.OK() {
		return 1
	}
	return 0
}

func runPublicKey(args []string) int {
	flags := flag.NewFlagSet("public-key", flag.ContinueOnError)
	testHex := flags.String("test-signing-public-key", "", "accept this throwaway key instead of the production key (tests)")
	if flags.Parse(args) != nil {
		return 2
	}
	testKey, err := parseTestKey(*testHex)
	if err != nil {
		logLine("membership public-key: " + err.Error())
		return 2
	}
	dir, err := privateDir()
	if err != nil {
		logLine("membership public-key: " + err.Error())
		return 1
	}
	config, err := membership.LoadConfig(dir)
	if err != nil {
		logLine("membership public-key: " + err.Error())
		return 1
	}
	public, err := membership.PublicHalf(config.SigningKey)
	if err != nil {
		logLine("membership public-key: " + err.Error())
		return 1
	}
	fmt.Println("license_public_key=" + hex.EncodeToString(public))
	if err := membership.CheckSigningKey(config.SigningKey, testKey); err != nil {
		logLine("membership public-key: " + err.Error())
		return 1
	}
	return 0
}
