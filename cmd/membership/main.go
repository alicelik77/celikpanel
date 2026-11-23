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
//	membership migrate-store -operator NAME
//	                        creates the store's tables (orders, payments, refunds, prices,
//	                        the audit log) and the licence columns they need; additive,
//	                        repeatable, never done at request time
//	membership reconcile -operator NAME
//	                        asks the payment provider about open orders older than ten
//	                        minutes and refunds in progress, applies what it confirms,
//	                        closes unpaid orders after a day, lists mismatches (a timer)
//	membership order | orders | test-account | price | extend | refund
//	                        the operator's store commands; each act is written to the
//	                        store's audit log with the operator's name and a reason.
//	                        refund is an exceptional act: there are no refunds by policy
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
	"context"
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
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/membership"
	"github.com/alicelik/celikpanel/internal/payment"
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
	case "migrate-store":
		os.Exit(runMigrateStore(os.Args[2:]))
	case "reconcile":
		os.Exit(runReconcile(os.Args[2:]))
	case "order", "orders", "test-account", "price", "extend", "refund":
		os.Exit(runOperator(os.Args[1], os.Args[2:]))
	}
	usage()
	os.Exit(2)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: membership cgi | serve [-listen 127.0.0.1:8379] [-test-signing-public-key HEX] | check-db [-test-signing-public-key HEX] | public-key [-test-signing-public-key HEX] | migrate-store -operator NAME [-test-signing-public-key HEX] | reconcile -operator NAME | order -id ORDER | orders [-pending] [-mismatch] | test-account -operator NAME -email ADDRESS -on|-off -reason TEXT | price -operator NAME -amount 99.00 [-from YYYY-MM-DD] -reason TEXT | extend -operator NAME -licence ID -days N -reason TEXT | refund -operator NAME -order ORDER -reason TEXT (each also [-test-signing-public-key HEX])")
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
	report.DatabaseIsAuthority = config.DatabaseIsAuthority
	report.Charging, report.PaymentProvider = config.Charging, config.PaymentProvider
	report.Payment = membership.PaymentState(payment.DefaultRegistry(), config)
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

func runMigrateStore(args []string) int {
	flags := flag.NewFlagSet("migrate-store", flag.ContinueOnError)
	testHex := flags.String("test-signing-public-key", "", "accept this throwaway key instead of the production key (tests)")
	operator := flags.String("operator", "", "who runs it (written to the store's audit log)")
	if flags.Parse(args) != nil || *operator == "" {
		logLine("membership migrate-store: name the operator: -operator NAME")
		return 2
	}
	testKey, err := parseTestKey(*testHex)
	if err != nil {
		logLine("membership migrate-store: " + err.Error())
		return 2
	}
	dir, config, err := loadChecked(testKey)
	if err != nil {
		logLine("membership migrate-store: " + err.Error())
		return 1
	}
	service, err := membership.OpenExisting(filepath.Join(dir, "members.sqlite"), config.SigningKey, nil, nil)
	if err != nil {
		logLine("membership migrate-store: " + err.Error())
		return 1
	}
	defer service.Close()
	if err := service.MigrateStore("operator:" + *operator); err != nil {
		logLine("membership migrate-store: " + err.Error())
		return 1
	}
	fmt.Printf("store version %d is ready\n", membership.StoreVersion)
	return 0
}

func runReconcile(args []string) int {
	flags := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	testHex := flags.String("test-signing-public-key", "", "accept this throwaway key instead of the production key (tests)")
	operator := flags.String("operator", "", "who runs it (written to the store's audit log)")
	if flags.Parse(args) != nil || *operator == "" {
		logLine("membership reconcile: name the operator: -operator NAME")
		return 2
	}
	testKey, err := parseTestKey(*testHex)
	if err != nil {
		logLine("membership reconcile: " + err.Error())
		return 2
	}
	dir, config, err := loadChecked(testKey)
	if err != nil {
		logLine("membership reconcile: " + err.Error())
		return 1
	}
	service, err := membership.OpenExisting(filepath.Join(dir, "members.sqlite"), config.SigningKey, nil, nil)
	if err != nil {
		logLine("membership reconcile: " + err.Error())
		return 1
	}
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	report, err := service.Reconcile(ctx, payment.DefaultRegistry(), config, "reconcile:"+*operator)
	if err != nil {
		logLine("membership reconcile: " + err.Error())
		return 1
	}
	fmt.Printf("confirmed %d, paid %d, closed unpaid %d, refunded %d, provider not reachable %d\n",
		report.Confirmed, report.Paid, report.Expired, report.Refunded, report.Unreachable)
	send := membership.ConfigMailer(config)
	for _, o := range report.PaidOrders {
		if err := service.SendReceipt(o, send); err != nil {
			fmt.Printf("receipt mail for order %s could not be sent; the receipt is on the member's page\n", o.ID)
		}
	}
	for _, o := range report.Mismatches {
		fmt.Printf("for the operator (mismatch, nothing granted): order %s\n", o.ID)
	}
	return 0
}

// runOperator is the operator's store commands. Reading commands need no name; acts need the
// operator's name and a reason, both written to the audit log.
func runOperator(command string, args []string) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	testHex := flags.String("test-signing-public-key", "", "accept this throwaway key instead of the production key (tests)")
	operator := flags.String("operator", "", "who acts (written to the audit log)")
	reason := flags.String("reason", "", "why (written to the audit log)")
	id := flags.String("id", "", "the order")
	orderID := flags.String("order", "", "the order")
	email := flags.String("email", "", "the member's e-mail address")
	on := flags.Bool("on", false, "flag as a test account")
	off := flags.Bool("off", false, "remove the test-account flag")
	pending := flags.Bool("pending", false, "only orders still open")
	mismatch := flags.Bool("mismatch", false, "only orders for the operator")
	amount := flags.String("amount", "", "the price, e.g. 99.00")
	from := flags.String("from", "", "the day the price applies from (UTC, YYYY-MM-DD; default now)")
	licence := flags.String("licence", "", "the licence id")
	days := flags.Int("days", 0, "days to add")
	if flags.Parse(args) != nil {
		return 2
	}
	acts := command != "order" && command != "orders"
	if acts && (*operator == "" || strings.TrimSpace(*reason) == "") {
		logLine("membership " + command + ": name the operator and the reason: -operator NAME -reason TEXT")
		return 2
	}
	testKey, err := parseTestKey(*testHex)
	if err != nil {
		logLine("membership " + command + ": " + err.Error())
		return 2
	}
	dir, config, err := loadChecked(testKey)
	if err != nil {
		logLine("membership " + command + ": " + err.Error())
		return 1
	}
	service, err := membership.OpenExisting(filepath.Join(dir, "members.sqlite"), config.SigningKey, nil, nil)
	if err != nil {
		logLine("membership " + command + ": " + err.Error())
		return 1
	}
	defer service.Close()
	if !service.StoreReady() {
		logLine("membership " + command + ": the store is not set up (membership migrate-store)")
		return 1
	}
	actor := "operator:" + *operator
	fail := func(err error) int {
		logLine("membership " + command + ": " + err.Error())
		return 1
	}
	switch command {
	case "order":
		o, err := service.GetOrder(*id)
		if err != nil {
			return fail(err)
		}
		printOrder(o)
		paid, _ := service.PaidAt(o.ID)
		fmt.Printf("payment confirmed at: %s\n", when(paid))
		refunds, _ := service.Refunds(o.ID)
		for _, r := range refunds {
			fmt.Printf("refund attempt %d: %s (%s)\n", r.Attempt, r.State, r.Outcome)
		}
		lines, _ := service.Audit(o.ID)
		for _, l := range lines {
			fmt.Printf("log %s %s %s: %s -> %s %s\n", when(l.At), l.Actor, l.Action, l.Before, l.After, l.Why)
		}
	case "orders":
		orders, err := service.Orders(0)
		if err != nil {
			return fail(err)
		}
		for _, o := range orders {
			open := o.State == membership.OrderCreated || o.State == membership.OrderPending
			if (*pending && !open) || (*mismatch && o.State != membership.OrderMismatch) {
				continue
			}
			printOrder(o)
		}
	case "test-account":
		member, err := service.MemberIDByEmail(*email)
		if err != nil || member == 0 || *on == *off {
			return fail(errors.New("name one member by -email and one of -on or -off"))
		}
		if err := service.SetTestAccount(actor, *reason, member, *on); err != nil {
			return fail(err)
		}
		fmt.Printf("member %d: test account %v\n", member, *on)
	case "price":
		minor, err := payment.ToMinor(*amount, "USD")
		if err != nil {
			return fail(errors.New("give the price as -amount 99.00"))
		}
		at := time.Now().Unix()
		if *from != "" {
			day, err := time.Parse("2006-01-02", *from)
			if err != nil {
				return fail(errors.New("give the day as -from YYYY-MM-DD"))
			}
			at = day.Unix()
		}
		if err := service.SetPrice(actor, *reason, "USD", minor, at); err != nil {
			return fail(err)
		}
		fmt.Printf("price %s USD from %s\n", *amount, when(at))
	case "extend":
		if err := service.ExtendByHand(actor, *reason, *licence, *days); err != nil {
			return fail(err)
		}
		fmt.Printf("licence %s extended by %d days\n", *licence, *days)
	case "refund":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		r, o, err := service.OperatorRefund(ctx, payment.DefaultRegistry(), config, actor, *orderID, *reason)
		fmt.Printf("refund attempt %d: %s (%s); order %s\n", r.Attempt, r.State, r.Outcome, o.State)
		if err != nil {
			return fail(err)
		}
	}
	return 0
}

func when(at int64) string {
	if at == 0 {
		return "-"
	}
	return time.Unix(at, 0).UTC().Format("2006-01-02 15:04 UTC")
}

func printOrder(o membership.Order) {
	mode := "test"
	if o.Live {
		mode = "live"
	}
	fmt.Printf("order %s: %s, licence %s, member %d, %d %s %s, %s, created %s", o.ID, o.Kind, o.LicenseID, o.Member, o.AmountMinor, o.Currency, mode, o.State, when(o.Created))
	if o.GrantedFrom != 0 {
		fmt.Printf(", term %s to %s", when(o.GrantedFrom), when(o.GrantedTo))
	}
	fmt.Println()
}
