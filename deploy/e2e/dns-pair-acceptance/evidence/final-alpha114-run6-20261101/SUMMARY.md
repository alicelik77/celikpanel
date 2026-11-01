# alpha.114 lab run 6, short final check (1 Nov 2026)

**Result: J1 fixed. Joined-server deletion works but reports the wrong reason (Medium).**

The owner chose a short run, about 1 hour, with two items:
- the J1 fix;
- deletion on the joined server.

The screens, S23 and the E-new-5 detail were not covered. The release notes say so.

- **Candidate:** `release/alpha114-integration` 69df35d0a.
- **Lab dist:** c155d9f41 on `lab/alpha114-label-r6`, local only. It is the candidate plus the version-114 lines; the budget check passed.

| Item | Result |
|---|---|
| Setup, topology A | **PASS**, after one retry from the wizard. The first attempt failed on both servers because the lab resolver briefly could not resolve the package mirrors (`service_install_failed`). This was a lab network issue. |
| J1, Debian 13 (Dovecot 2.4.1) | **PASS**: 2 delivered, 1 reply. After the `lmtp_rejected` refusal, mail is delivered with 0 bounces. After restore, 1 reply. |
| J1, Ubuntu 24.04 (Dovecot 2.3.21) | **PASS**, with the same sequence on the joined server. |
| E-new-4 (a) | Deletion finished by itself in 21 s and both DNS servers refuse the zone. **Defect K1:** the DELETE answered `remote_dns_main_unreachable` while the main panel was running. |
| E-new-4 (b) | Deletion finished by itself about 3 min after the main panel started. **Defect K1:** the 202 had no reason, and the status said `confirming` for 7 s while the main panel was stopped. |
| E-new-4 (c) | The deletion survived the joined server's reboot and finished by itself. Same reason defect as (b). |
| Status after deletion | **Defect K1:** the status read answers 404, not 204. Domains read 404 as "could not be read". |

## K1 (Medium): wrong or missing deletion reasons
- **(a) Slow answer reported as unreachable.** The main server's peer proof took 18.4 s, longer than the joined server's 10 s wait for an answer.
  - The timeout became `errRemoteDNSPublicationPending`, which was mapped to `remote_dns_main_unreachable`.
  - **Fix:** the transport records whether the whole request reached the main server (`remoteDNSExchangeError.Delivered`). If it did, the reason is `remote_dns_removal_confirming`.
- **(b) No reason.** A bare "remote DNS HTTPS exchange failed" was not classified.
  - **Fix:** it is now the typed exchange error. With no delivery, the reason is `remote_dns_main_unreachable`, and the DELETE answer records it at once.
- **404.** The screen now treats 404 as a finished deletion and reads the list again, so the row leaves by itself.

## Files
- Evidence tarball SHA-256: `51dae337c5e72772a1c4c23243bc56474d752b6b324e3229d59a8dcf9e01406f`
- Dist SHA-256: `807b4d1e29adce6147d649ca325a3ce926f4783649b75bdaa849dfded17dbf03`
- Secret scan: clean. 788 + 17 files checked against 28 secrets.
- Harness: `beta1/a114/run6/`. Lab commit c97e21eca.
