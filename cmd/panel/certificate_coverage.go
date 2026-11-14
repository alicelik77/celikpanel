package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

// What a domain's certificate protects (alpha.134, docs/CERTIFICATE-COVERAGE.md).
//
// The owner asked, looking at his certificate screen: "how are we supposed to
// know what it covers and what it does not?" This file answers it as one list
// of everything of the domain that a visitor or a mail program connects to:
// what it is, the name, and whether a trusted certificate protects it (which
// one, until when) or why not, with at most one action that fixes it.
//
//	GET /api/v1/domains/{id}/ssl/coverage   the list (reads only; the same
//	                                        gate as the certificate screen)
//
// The decision is certificateCoverage, a pure function of what was read, so
// every line and reason is tested in a table. The mail screen states its
// "mail programs" line from the same function (mailProgramsCoverage).
//
// Alan adının sertifikasının neyi koruduğu: ziyaretçinin ya da posta
// programının bağlandığı her şey, korunuyor mu, hangi sertifikayla ve ne zamana
// dek, korunmuyorsa neden ve tek düzeltme eylemi. Karar saf bir işlevdir.

// The kinds of line, in the order of the list.
const (
	coverageKindSite         = "site"
	coverageKindWebmail      = "webmail"
	coverageKindMailPrograms = "mail_programs"
	coverageKindMailName     = "mail_name"
	coverageKindMailSetup    = "mail_setup"
)

// States: protected, not protected (with a reason) or not known now.
const (
	coverageProtected    = "protected"
	coverageNotProtected = "not_protected"
	coverageUnknown      = "unknown"
)

// Which certificate protects a line.
const (
	coverageByDomain     = "domain_certificate"
	coverageByServerMail = "server_mail_certificate"
	coverageByWebmail    = "webmail_certificate"
)

// Reasons, each one sentence on the screen (certcover. texts).
const (
	coverageReasonNoCertificate      = "no_certificate"
	coverageReasonNotInCertificate   = "not_in_certificate"
	coverageReasonPointsElsewhere    = "points_elsewhere"
	coverageReasonNoAddress          = "no_address"
	coverageReasonNotUsable          = "certificate_not_usable"
	coverageReasonMailNotUsing       = "mail_not_using"
	coverageReasonServerMailNotReady = "server_certificate_not_ready"
	coverageReasonMailNameUnknown    = "mail_name_unknown"
	coverageReasonWebmailNoCert      = "webmail_no_certificate"
	coverageReasonNotChecked         = "not_checked"
)

// Actions: at most one per line.
const (
	// coverageActionReissue: request the certificate again; every name of
	// the site and every name that points here (mail.<domain> included) is
	// in the new one.
	coverageActionReissue = "reissue"
	// coverageActionCoverMail: the same request, offered on the mail name's
	// line ("Posta adını da kapsa"); mail then uses it for that name.
	coverageActionCoverMail = "cover_mail"
	// coverageActionUpload: an uploaded certificate cannot be requested
	// again; a new one is uploaded.
	coverageActionUpload = "upload"
	// coverageActionUseForMail: the certificate covers mail.<domain> but mail
	// does not serve it yet.
	coverageActionUseForMail = "use_for_mail"
	// coverageActionDNS: the domain's DNS records on this server.
	coverageActionDNS = "dns"
	// coverageActionMailSetup: the mail screen adds the missing auto-setup
	// records.
	coverageActionMailSetup = "mail_setup"
	// coverageActionRecheck: the name waits for a DNS change made elsewhere
	// (the zone is not on this server); the list asks the public DNS again.
	// An unmet prerequisite, not a failure: nothing is requested from a
	// certificate authority for a name that does not point here.
	coverageActionRecheck = "recheck"
)

// dnsPrerequisiteAction is the one action of a name that does not point
// here: this server's DNS records when the zone is here, otherwise a new
// look at the public DNS once whoever runs the zone has changed it.
func (in coverageInput) dnsPrerequisiteAction() string {
	if in.DNSHere {
		return coverageActionDNS
	}
	return coverageActionRecheck
}

// coverageLine is one line of the list.
type coverageLine struct {
	Kind string `json:"kind"`
	// Role of a site name: domain, www or alias.
	Role   string   `json:"role,omitempty"`
	Name   string   `json:"name"`
	State  string   `json:"state"`
	By     string   `json:"by,omitempty"`
	Until  string   `json:"until,omitempty"`
	Reason string   `json:"reason,omitempty"`
	IPs    []string `json:"ips,omitempty"`
	Action string   `json:"action,omitempty"`
}

// coverageCertificate is the domain's active certificate as read now.
type coverageCertificate struct {
	Names     []string
	ExpiresAt time.Time
	// WebUsable: the web server serves it and it is valid and trusted.
	WebUsable bool
	// Trusted: valid and trusted (mail does not depend on the web server).
	Trusted bool
	// Uploaded: a certificate the owner uploaded; it cannot be requested again.
	Uploaded bool
	// ServesMail: mail serves it for mail.<domain> (ssl_certificates.secure_mail).
	ServesMail bool
}

// coverageServerMail is the server's own mail certificate: ready, pending
// (not obtained yet) or unknown.
type coverageServerMail struct {
	State     string
	ExpiresAt time.Time
}

// coverageWebmail is what this server's web server answers for web mail's
// address: secure yes, no or unknown, with the certificate's end.
type coverageWebmail struct {
	Host      string
	Secure    string
	ExpiresAt time.Time
}

// coverageInput is everything the list is decided from.
type coverageInput struct {
	Domain    string
	SiteNames []string
	// CertificateKnown false: the certificate could not be read now.
	CertificateKnown bool
	Certificate      *coverageCertificate
	MailHere         bool
	// ServerMailHost is the server's mail name ("" when it is not known).
	ServerMailHost string
	// ToldHost is the name mail programs are told (mailClientHost).
	ToldHost   string
	ServerMail coverageServerMail
	Webmail    *coverageWebmail
	// SetupNames: autoconfig.<domain>, autodiscover.<domain> when the
	// domain's site answers mail programs.
	SetupNames []string
	// DNS: where each name resolves (lower-case name → check).
	DNS map[string]certificateNameCheck
	// DNSHere: the domain's zone is on this server.
	DNSHere bool
}

func coverageNormal(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

func coverageTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// certificateCoverage decides the list. Order: the site's names, web mail,
// mail programs, mail.<domain>, the auto-setup names.
func certificateCoverage(in coverageInput) []coverageLine {
	domain := coverageNormal(in.Domain)
	lines := []coverageLine{}
	for _, name := range in.SiteNames {
		role := "alias"
		switch coverageNormal(name) {
		case domain:
			role = "domain"
		case "www." + domain:
			role = "www"
		}
		line := in.nameLine(coverageKindSite, name, false)
		line.Role = role
		lines = append(lines, line)
	}
	if !in.MailHere {
		return lines
	}
	if in.Webmail != nil {
		lines = append(lines, webmailLine(*in.Webmail))
	}
	lines = append(lines, in.mailProgramsLine())
	mailName := "mail." + domain
	if coverageNormal(in.ToldHost) != mailName {
		if line, shown := in.mailNameLine(mailName); shown {
			lines = append(lines, line)
		}
	}
	for _, name := range in.SetupNames {
		lines = append(lines, in.nameLine(coverageKindMailSetup, name, true))
	}
	return lines
}

// nameLine: a name the domain's certificate is meant to carry (a site name
// or an auto-setup name), served by the web server.
func (in coverageInput) nameLine(kind, name string, setup bool) coverageLine {
	line := coverageLine{Kind: kind, Name: name}
	switch {
	case !in.CertificateKnown:
		line.State, line.Reason = coverageUnknown, coverageReasonNotChecked
		return line
	case in.Certificate == nil:
		line.State, line.Reason = coverageNotProtected, coverageReasonNoCertificate
		return line
	}
	cert := in.Certificate
	if certificateCoversHostname(cert.Names, name) {
		if cert.WebUsable {
			line.State, line.By, line.Until = coverageProtected, coverageByDomain, coverageTime(cert.ExpiresAt)
		} else {
			line.State, line.Reason = coverageNotProtected, coverageReasonNotUsable
		}
		return line
	}
	line.State = coverageNotProtected
	check, known := in.DNS[coverageNormal(name)]
	switch {
	case known && check.State == certificateNameElsewhere && len(check.IPs) > 0:
		line.Reason, line.IPs = coverageReasonPointsElsewhere, append([]string(nil), check.IPs...)
		line.Action = in.dnsPrerequisiteAction()
	case known && check.State == certificateNameElsewhere:
		line.Reason = coverageReasonNoAddress
		line.Action = in.dnsPrerequisiteAction()
		if in.DNSHere && setup {
			line.Action = coverageActionMailSetup
		}
	default:
		// It points here, or where it points is not known: the request
		// checks again and includes it only if it points here.
		line.Reason = coverageReasonNotInCertificate
		line.Action = coverageActionReissue
		if cert.Uploaded {
			line.Action = coverageActionUpload
		}
	}
	return line
}

// mailProgramsLine: the name mail programs are told, whichever it is.
func (in coverageInput) mailProgramsLine() coverageLine {
	line := coverageLine{Kind: coverageKindMailPrograms, Name: in.ToldHost}
	told := coverageNormal(in.ToldHost)
	switch {
	case told == "" || in.ServerMailHost == "":
		line.State, line.Reason = coverageNotProtected, coverageReasonMailNameUnknown
	case told == coverageNormal(in.ServerMailHost):
		switch in.ServerMail.State {
		case "ready":
			line.State, line.By, line.Until = coverageProtected, coverageByServerMail, coverageTime(in.ServerMail.ExpiresAt)
		case "pending":
			line.State, line.Reason = coverageNotProtected, coverageReasonServerMailNotReady
		default:
			line.State, line.Reason = coverageUnknown, coverageReasonNotChecked
		}
	case !in.CertificateKnown:
		line.State, line.Reason = coverageUnknown, coverageReasonNotChecked
	case in.Certificate != nil && in.Certificate.ServesMail && in.Certificate.Trusted &&
		certificateCoversHostname(in.Certificate.Names, in.ToldHost):
		line.State, line.By, line.Until = coverageProtected, coverageByDomain, coverageTime(in.Certificate.ExpiresAt)
	default:
		line.State, line.Reason = coverageNotProtected, coverageReasonNotUsable
	}
	return line
}

// mailNameLine: mail.<domain>, which the MX and hand-typed settings use. It
// is shown when it is in the certificate or the public DNS has it (or could
// not say); a domain without the name has no line for it.
func (in coverageInput) mailNameLine(mailName string) (coverageLine, bool) {
	line := coverageLine{Kind: coverageKindMailName, Name: mailName}
	check, known := in.DNS[mailName]
	covered := in.CertificateKnown && in.Certificate != nil && certificateCoversHostname(in.Certificate.Names, mailName)
	if !covered && known && check.State == certificateNameElsewhere && len(check.IPs) == 0 {
		return line, false
	}
	switch {
	case !in.CertificateKnown:
		line.State, line.Reason = coverageUnknown, coverageReasonNotChecked
	case covered && !in.Certificate.Trusted:
		line.State, line.Reason = coverageNotProtected, coverageReasonNotUsable
	case covered && in.Certificate.ServesMail:
		line.State, line.By, line.Until = coverageProtected, coverageByDomain, coverageTime(in.Certificate.ExpiresAt)
	case covered:
		line.State, line.Reason, line.Action = coverageNotProtected, coverageReasonMailNotUsing, coverageActionUseForMail
	case !known || check.State == certificateNameUnknown:
		line.State, line.Reason = coverageUnknown, coverageReasonNotChecked
	case check.State == certificateNameElsewhere:
		line.State, line.Reason, line.IPs = coverageNotProtected, coverageReasonPointsElsewhere, append([]string(nil), check.IPs...)
		line.Action = in.dnsPrerequisiteAction()
	case in.Certificate == nil:
		line.State, line.Reason = coverageNotProtected, coverageReasonNoCertificate
	default:
		line.State, line.Reason, line.Action = coverageNotProtected, coverageReasonNotInCertificate, coverageActionCoverMail
		if in.Certificate.Uploaded {
			line.Action = coverageActionUpload
		}
	}
	return line, true
}

func webmailLine(webmail coverageWebmail) coverageLine {
	line := coverageLine{Kind: coverageKindWebmail, Name: webmail.Host}
	switch webmail.Secure {
	case "yes":
		line.State, line.By, line.Until = coverageProtected, coverageByWebmail, coverageTime(webmail.ExpiresAt)
	case "no":
		line.State, line.Reason = coverageNotProtected, coverageReasonWebmailNoCert
	default:
		line.State, line.Reason = coverageUnknown, coverageReasonNotChecked
	}
	return line
}

// ---- Reading what the list is decided from ----------------------------------

// coverageProbeTLS reads, on this server, which certificate the web server
// answers for a name: whether it is for that name and unexpired, and its end.
// Swapped by tests.
var coverageProbeTLS = func(ctx context.Context, host string) (string, time.Time) {
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 3 * time.Second}, Config: &tls.Config{ServerName: host, InsecureSkipVerify: true}}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(probeCtx, "tcp", "127.0.0.1:443")
	if err != nil {
		return "unknown", time.Time{}
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return "no", time.Time{}
	}
	leaf := state.PeerCertificates[0]
	if leaf.VerifyHostname(host) != nil || !time.Now().Before(leaf.NotAfter) {
		return "no", time.Time{}
	}
	return "yes", leaf.NotAfter
}

// coverageCheckNames asks the public DNS where the names point. Swapped by
// tests through certificateNameResolvers.
func coverageCheckNames(ctx context.Context, names []string) map[string]certificateNameCheck {
	checkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out := map[string]certificateNameCheck{}
	for _, check := range checkCertificateNames(checkCtx, names, "") {
		out[coverageNormal(check.Name)] = check
	}
	return out
}

// coverageCertificateOf reads the domain's active certificate. known is false
// when it could not be read now (the Agent did not answer).
func (p *Panel) coverageCertificateOf(ctx context.Context, domainID int, managed []string) (*coverageCertificate, bool, error) {
	var (
		certType string
		secure   int
	)
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT type, COALESCE(secure_mail, 0) FROM ssl_certificates
		WHERE domain_id = ? AND status = 'active'
		ORDER BY created_at DESC LIMIT 1`, domainID).Scan(&certType, &secure)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	runtime, err := p.loadCertificateRuntimeStatus(ctx, domainID, managed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	info := runtime.Info
	if info.Error != "" && !info.Valid && len(info.DNSNames) == 0 {
		return nil, false, nil
	}
	trusted := info.Valid && info.TrustChecked && info.Trusted
	return &coverageCertificate{
		Names:      append([]string(nil), info.DNSNames...),
		ExpiresAt:  info.ExpiresAt,
		WebUsable:  trusted && runtime.Activated && runtime.SitePathsMatch && !runtime.ActivationPending,
		Trusted:    trusted,
		Uploaded:   certType != "letsencrypt",
		ServesMail: secure == 1,
	}, true, nil
}

// coverageServerMailOf is the server's mail certificate, from the
// certificates overview's own cached read of the Agent.
func (p *Panel) coverageServerMailOf(ctx context.Context, host string) coverageServerMail {
	for _, view := range p.agentCertificateViews(ctx, time.Now()) {
		if view.Kind != "mail_host" || coverageNormal(view.Name) != coverageNormal(host) {
			continue
		}
		switch view.State {
		case "in_use":
			at, _ := time.Parse(time.RFC3339, view.ExpiresAt)
			return coverageServerMail{State: "ready", ExpiresAt: at}
		case "pending":
			return coverageServerMail{State: "pending"}
		}
	}
	return coverageServerMail{State: "unknown"}
}

// coverageMailInputs fills the mail part of the input: whether mail is here,
// the server's mail name, the name programs are told and the server's mail
// certificate.
func (p *Panel) coverageMailInputs(ctx context.Context, domainID int, domain string, in *coverageInput) error {
	in.MailHere = mailAutoconfigMailHere(p, ctx)
	if !in.MailHere {
		return nil
	}
	server, err := p.mailTLSHostname(ctx)
	if err != nil {
		return nil // the mail programs line says the name is not set
	}
	in.ServerMailHost = server
	told, err := p.mailClientHost(ctx, domainID, domain, server)
	if err != nil {
		return err
	}
	in.ToldHost = told
	in.ServerMail = p.coverageServerMailOf(ctx, server)
	return nil
}

// gatherCertificateCoverage reads everything the list needs. It only reads:
// the panel's database, the Agent's certificate reads, the public DNS and
// this server's own web server.
func (p *Panel) gatherCertificateCoverage(ctx context.Context, domainID int) (coverageInput, error) {
	domain, err := p.domainNameByIDStrict(ctx, domainID)
	if err != nil {
		return coverageInput{}, err
	}
	in := coverageInput{Domain: coverageNormal(domain), CertificateKnown: true}
	var siteCount int
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sites WHERE domain_id = ? AND COALESCE(project_type, '') <> 'dnsonly'`,
		domainID).Scan(&siteCount); err != nil {
		return coverageInput{}, err
	}
	managed, err := p.managedSiteHostnames(ctx, domainID)
	if err != nil {
		return coverageInput{}, err
	}
	if siteCount > 0 {
		in.SiteNames = managed
	}
	in.Certificate, in.CertificateKnown, err = p.coverageCertificateOf(ctx, domainID, managed)
	if err != nil {
		return coverageInput{}, err
	}
	if err := p.coverageMailInputs(ctx, domainID, in.Domain, &in); err != nil {
		return coverageInput{}, err
	}
	names := append([]string(nil), in.SiteNames...)
	if in.MailHere {
		if plan, err := p.mailAutoconfigFor(ctx, domainID); err == nil && plan.available {
			in.SetupNames = plan.names
		}
		names = append(names, "mail."+in.Domain)
		names = append(names, in.SetupNames...)
		if address, ok := p.webmailAddress(ctx); ok {
			secure, until := coverageProbeTLS(ctx, address.Host)
			in.Webmail = &coverageWebmail{Host: address.Host, Secure: secure, ExpiresAt: until}
		}
	}
	in.DNS = coverageCheckNames(ctx, names)
	if mode, err := p.domainDNSManagementMode(ctx, in.Domain); err == nil && mode == setupDNSModeLocal {
		in.DNSHere = true
	}
	return in, nil
}

// mailProgramsCoverage is the mail screen's statement under the server name:
// the list's "mail programs" line, read without asking the public DNS. nil
// when mail is not on this server.
func (p *Panel) mailProgramsCoverage(ctx context.Context, domainID int, domain string) *coverageLine {
	in := coverageInput{Domain: coverageNormal(domain), CertificateKnown: true}
	managed, err := p.managedSiteHostnames(ctx, domainID)
	if err != nil {
		return nil
	}
	if in.Certificate, in.CertificateKnown, err = p.coverageCertificateOf(ctx, domainID, managed); err != nil {
		return nil
	}
	if err := p.coverageMailInputs(ctx, domainID, in.Domain, &in); err != nil || !in.MailHere {
		return nil
	}
	line := in.mailProgramsLine()
	return &line
}

// handleCertificateCoverage answers the list. GET only.
func (p *Panel) handleCertificateCoverage(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	in, err := p.gatherCertificateCoverage(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"lines":     certificateCoverage(in),
		"mail_name": "mail." + in.Domain,
		// server_ip: the address a name must point to, for the next step of
		// a name that points elsewhere ("" when it is not known).
		"server_ip":  coverageServerIPv4(certificateServerAddresses()),
		"checked_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// coverageServerIPv4 is the first IPv4 address of this server: the one an A
// record names. "" when the server has none known.
func coverageServerIPv4(addresses []string) string {
	for _, address := range addresses {
		if ip := net.ParseIP(address); ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}
