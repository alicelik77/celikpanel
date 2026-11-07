package transport

// Certificate requests (alpha.128, G31): a certificate signing request (CSR)
// for a certificate bought elsewhere. Agent.CertificateRequestCreateV1 makes
// a private key in Go and a CSR for it; the key is written root-only under
// the Agent's private state directory and never leaves the Agent: only the
// CSR, the SHA-256 of the public key and the key file's path are answered.
// Agent.CertificateRequestDiscardV1 removes such a key file.
//
// Sertifika isteği: özel anahtar Go ile üretilir, yalnız root okuyabilir ve
// Agent'tan hiç çıkmaz; yanıtta yalnız CSR ve açık anahtarın özeti vardır.

// Certificate request key types; the first is the default.
const (
	CertificateKeyECDSAP256 = "ecdsa-p256"
	CertificateKeyRSA2048   = "rsa-2048"
	CertificateKeyRSA3072   = "rsa-3072"
	CertificateKeyRSA4096   = "rsa-4096"
)

// ValidCertificateRequestKeyType names the key types a request may use.
func ValidCertificateRequestKeyType(keyType string) bool {
	switch keyType {
	case CertificateKeyECDSAP256, CertificateKeyRSA2048, CertificateKeyRSA3072, CertificateKeyRSA4096:
		return true
	}
	return false
}

type CertificateRequestCreateRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	// Token names the key file: 32 lowercase hexadecimal characters the
	// panel makes; a second call with the same token is refused, never a
	// second key.
	Token string `json:"token"`
	// CommonName is the first of Names; Names are canonical host names
	// (a leading "*." for a wildcard), at most 50.
	CommonName string   `json:"common_name"`
	Names      []string `json:"names"`
	KeyType    string   `json:"key_type"`
}

type CertificateRequestCreateResponse struct {
	CSRPEM          string `json:"csr_pem,omitempty"`
	PublicKeySHA256 string `json:"public_key_sha256,omitempty"`
	// KeyFile is the absolute path of the root-only key file, kept by the
	// panel so a later install or discard names the same file. Never its
	// content.
	KeyFile string `json:"key_file,omitempty"`
	Error   string `json:"error,omitempty"`
}

type CertificateRequestDiscardRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit"`
	Token               string `json:"token"`
}

type CertificateRequestDiscardResponse struct {
	// Removed: the key file was there and is gone; false with no Error: it
	// was already gone.
	Removed bool   `json:"removed"`
	Error   string `json:"error,omitempty"`
}
