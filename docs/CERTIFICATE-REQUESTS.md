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
  The row is in `certificate_requests` (migration 072). *Discard* removes the
  key (`Agent.CertificateRequestDiscardV1`) and marks the row discarded.
- The key file is the owner's: with or without CelikPanel it can be used with
  the issued certificate in the owner's own configuration (D-022).

## Installing the issued certificate (alpha.128 follow-up)

On a waiting request, *Install the issued certificate…* takes the certificate
the authority sent (PEM; intermediates may follow it) and the site it is for.
`POST /api/v1/maintenance/certificate-requests/{id}/install`
`{"target": "site", "domain_id", "certificate", "chain"}` (administrator):

1. Before anything changes the panel proves, from the certificate alone, that
   `sha256(RawSubjectPublicKeyInfo)` is the request's recorded fingerprint
   (else `CERTIFICATE_REQUEST_KEY_MISMATCH`), that it is valid now (else
   `CERTIFICATE_REQUEST_CERT_NOT_VALID_NOW`, with the date) and that it covers
   every requested name (else `CERTIFICATE_REQUEST_NAMES_MISSING`, naming
   them). Only certificates are accepted in the body; a private key is
   refused. An installed or discarded request is refused.
2. The site's own custom-certificate path then runs unchanged
   (`installCustomCertificateForDomain`, split out of the upload handler):
   Agent validation (chain, dates, names, key match), managed host names, the
   replacement guard, installation, activation, the web server and mail, with
   the same steps in the progress window and the same audit. The only
   difference is the key's source: `Agent.ValidateCertificate` and
   `Agent.InstallCustomCertificate` receive `request_key_token` and
   `request_key_sha256` instead of a key, and the Agent reads the key from
   its own file (`O_NOFOLLOW`, regular, 0600, owned by the Agent, inside the
   0700 directory) and proves its public key is the recorded one. Sending a
   key and naming a request together is refused. The key never passes
   through the panel or the browser.
3. On success the request becomes `installed` (`installed_for =
   site:<name>`) and the audit has `certificate.request.install`. The site's
   installed files hold their own copy of the key, so a later Discard cannot
   break the site.

Targets: a site. The panel's own certificate and the mail server's host
certificate are renewed by Let's Encrypt through their own lineages; a bought
certificate would be replaced at the next renewal there, so the panel refuses
those targets (`CERTIFICATE_REQUEST_TARGET_UNSUPPORTED`) and says to install
it on the site, whose domain's mail then uses it too.

## Evidence

Agent tests: key and CSR for ECDSA and RSA, CSR signature and fingerprint,
0700/0600 modes, no second key for a token, refused input, refused unsafe
directory, build gate, discard twice. Panel tests: overview (sources,
renewal, used by, cached Agent reads, a non-boolean stored renewal), create
(canonical names, no key in the answer, audit), discard, unknown Agent
answer, administrator only.
