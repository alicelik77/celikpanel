package transport

import "time"

const AgentCapabilityMailHostCertificateV1 = "mail_host_certificate_v1"

type IssueMailHostCertificateRequest struct {
	ServiceMutationBinding
	Domain              string `json:"domain"`
	Email               string `json:"email"`
	ExpectedBuildCommit string `json:"expected_build_commit"`
}

type IssueMailHostCertificateResponse struct {
	Issued    bool      `json:"issued"`
	ExpiresAt time.Time `json:"expires_at"`
	// ErrorCode / ClockSkewSeconds: CertificateClockSkewErrorCode when the
	// issued leaf is not valid yet by this server's clock (additive).
	ErrorCode        string `json:"error_code,omitempty"`
	ClockSkewSeconds int    `json:"clock_skew_seconds,omitempty"`
	Error            string `json:"error,omitempty"`
}

// CertificateClockSkewErrorCode: a certificate the setup issued is not valid
// yet by this server's clock (the clock is behind the authority's). Shared by
// every certificate the setup issues (Beta-1 third re-run N2/N4).
const CertificateClockSkewErrorCode = "certificate_clock_skew"

type MailHostCertificateStatusRequest struct {
	Domain string `json:"domain"`
}

type MailHostCertificateStatusResponse struct {
	Ready        bool      `json:"ready"`
	Domain       string    `json:"domain"`
	ExpiresAt    time.Time `json:"expires_at"`
	RenewalReady bool      `json:"renewal_ready"`
	Error        string    `json:"error,omitempty"`
}
