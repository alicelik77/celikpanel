# Certificate requests (CSR) — design note (alpha.128, G31)

## What ships in alpha.128

- Server → Maintenance → *Certificates on this server*: one read-only list of
  the panel's certificate, the default HTTPS server, the mail host and every
  site's active certificate, with "used by", issuer, days left and whether it
  renews automatically. Let's Encrypt renewal stays automatic; there is no
  switch here.
- *Certificate requests*: the administrator enters host names (first = common
  name, `*.` wildcards allowed, at most 50) and a key type (ECDSA P-256
  default; RSA 2048/3072/4096). `Agent.CertificateRequestCreateV1` makes the
  key in Go (`crypto/ecdsa`, `crypto/rsa`, `x509.CreateCertificateRequest`; no
  openssl, no shell), writes it once as PKCS#8 PEM to
  `/var/lib/celikpanel-agent-private/certificate-requests/<token>.key`
  (directory 0700, file 0600, root; temporary file + `link`, never replaced)
  and answers only the CSR, the SHA-256 of the public key and the file's path.
  The row is in `certificate_requests` (migration 069). *Discard* removes the
  key (`Agent.CertificateRequestDiscardV1`) and marks the row discarded.
- The key file is the owner's: with or without CelikPanel it can be used with
  the issued certificate in the owner's own configuration (D-022).

## What does not ship: installing the issued certificate

Not in alpha.128, on purpose. The existing custom-certificate path
(`handleUploadCertificate` → `Agent.ValidateCertificate` →
`Agent.InstallCustomCertificate`) receives the private key *from the panel*,
which a request's key must never pass through. Routing it safely needs:

1. A new typed RPC, e.g. `Agent.InstallRequestedCertificateV1{domain, token,
   cert_pem, chain_pem, expected_public_key_sha256}`. The Agent reads the key
   from its own file (`O_NOFOLLOW`, regular, 0600, root-owned, inside the key
   directory), proves `sha256(cert.RawSubjectPublicKeyInfo)` equals the stored
   fingerprint and that the key matches the certificate
   (`certificateMatchesPrivateKey`), then runs the same validation
   (`ValidateCertificate`: chain, dates, names) and the same publication
   (`publishCertificateVersion`) as `InstallCustomCertificate`.
2. A panel handler that shares the body of the custom upload after its
   validation step — managed host names covered
   (`managedSiteHostnames`), `validateCustomCertificateReplacement`,
   `activateCertificate`, `applyVhostForDomain`, `syncCertificateDependents`,
   the cleanup of an uncommitted snapshot and the SSL lock — so a requested
   certificate behaves exactly like an uploaded one afterwards. The upload
   handler is one long function today; it has to be split into "validate
   inputs" and "install a validated certificate" first, with its tests.
3. Matching on upload: the administrator uploads the issued certificate on the
   site's SSL page; the panel finds the waiting request whose
   `public_key_sha256` equals the certificate's, and only that request's key
   is used. `certificate_requests.state` becomes `installed` with
   `installed_at`/`installed_for` (the columns exist in 069), and *Discard* is
   refused while it is in use (already enforced).
4. The panel's own certificate and the mail host certificate are Let's
   Encrypt-only today; a requested certificate is for sites only.

Until then the screen says plainly that installing from the panel is not
available yet and that the key file can be used with the owner's own tools.

## Evidence

Agent tests: key and CSR for ECDSA and RSA, CSR signature and fingerprint,
0700/0600 modes, no second key for a token, refused input, refused unsafe
directory, build gate, discard twice. Panel tests: overview (sources,
renewal, used by, cached Agent reads, a non-boolean stored renewal), create
(canonical names, no key in the answer, audit), discard, unknown Agent
answer, administrator only.
