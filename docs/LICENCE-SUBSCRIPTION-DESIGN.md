# Paid yearly licences: technical design

*Design document, 14 November 2026, for the owner's licence, subscription and payment
direction of the same day ([D-047](DECISIONS.md)). Status: design only. Nothing here is
implemented; no installed panel, live licence service or payment provider was contacted
or changed. Line references are to `main` at `1f1d80672`.*

> **Amendment, 23 November 2026 ([D-047](DECISIONS.md), the owner's words there).** The
> owner's words of 23 November establish that members manage their accounts and
> subscriptions and that Stripe is the first payment method: section e's provider adapter is
> built for Stripe first (every purchase a one-time Checkout payment; a Stripe subscription
> only when the member turns automatic renewal on, as point 6 of D-047 already requires),
> behind the same provider-neutral seam; building against a fake Stripe server first and then
> Stripe's test mode is the plan's proposal, not the owner's words. By the owner's direction of 15 November 2026 (no new PHP), the service in section e
> is written in Go and replaces `portal-membership/` step by step: first a Go service at
> exact parity with the PHP one (same pages and texts, same panel API, the same SQLite file
> opened in place, the same signing key), then orders and payments. Keeping the PHP release
> selectable as the way back during the switch is the plan's proposal, not the owner's
> decision. Where this document says "the service" or names `Service.php`/`http.php`, read
> the Go service for new work; the PHP lines stay the reference for parity. The owner's
> requirements of the same day (celikpanel.net moves to Frankfurt; the licence server can be
> changed at any time and need not be celikpanel.net), his acceptance in principle of a
> signed operator licence, and the design proposals that are not decided are recorded in
> D-047, each with its source, and are not designed here. A draft procedure for moving or
> switching the member service, not yet rehearsed, is in §e ("Moving or switching the member
> service"). Still open: the
> legal entity and country of the Stripe account, tax and invoices, the founder price,
> currencies other than USD (§k). Everything else here is unchanged.

> **Changed by the owner on 23 November 2026 ([D-047](DECISIONS.md), his words quoted there):
> no grace period, no protection state, no refunds.** "ek süre diye birşey yok. 1 ay
> öncesinden uyarı yapalım. uzatsın yada sonucuna katlansın."; "lisansın bittiği gün biter.
> yeni lisans alması gerekir."; "lisans yoksa panel çalışmaz. sunucu çalışır bizi
> ilgilendirmez"; "hiçbir zaman iade yok. ilk seferde 30 gün trial var isterse. ödeme iadesi
> yok." What this document says below is superseded where it disagrees:
>
> - **Dropped by this decision:** the 30 days of grace, the protection level and its gate
>   (main choices 1, 2 and 4; §a's grace and protection states; §b entirely; the parts of §c
>   and §g that carry grace or protection into the receipt and the texts; the stage of §j
>   that builds them). A licence ends on its end date; without a licence the panel's screens
>   are closed until a licence is entered again, and the server -- hosted sites, mail, DNS,
>   databases, scheduled jobs, certificate renewal -- keeps running. The receipt format work
>   planned for grace and protection is no longer needed. The warnings stay at 30, 7 and 1 day
>   before the end.
> - **Unchanged:** the 30-day offline window when CelikPanel's own licence service cannot be
>   reached (main choice 4's other half, D-039) is a different thing and stays.
> - **Refunds (§e "Refund", the operator's `refund`):** there are none. A refund or a lost
>   dispute that happens at the payment provider (the operator's exceptional act, or a
>   chargeback) is still applied when confirmed: a first year is revoked (the generation
>   raised, "Revocations raise the generation" below), a renewal's year is taken back. No page
>   offers a refund; the page before payment says plainly that payments are not refunded and
>   that the 30-day trial is for trying first.
> - **As built in S2 (after its two reviews):**
>   - *Who buys.* An account that buys its years (in test mode: one the operator flagged; live
>     mode is refused in this release) gets its first licence in that mode as the 30-day
>     trial, without payment details -- unless it held a free beta licence, which was its free
>     period. One trial per account and per mode, and per server: a trial licence does not
>     activate on a server that already ran a trial (the coordinator's anti-abuse rules, for
>     the owner to confirm). Later licences wait for their first year (at most three at a
>     time; their end is their creation time, so neither the Go nor the PHP service activates
>     them); the order is made only when the member asks to pay. No free renewal for such an
>     account. Everyone else keeps the beta's free licence and free renewal.
>   - *Payments made in time are honoured.* A renewal at the locked price could only be paid
>     before the licence's end (its checkout ended then), so it is granted from the old end
>     whenever its confirmation arrives. The price stays locked only while the licence is in
>     paid time without a gap.
>   - *Disputes.* While a dispute is open nothing changes for the customer; reconciliation asks
>     again every day until it closes, whatever the payment's age. A lost dispute (and a
>     refund at the provider) takes back exactly the time that payment gave; later paid years
>     stay; if nothing remains, the licence is ended (revoked, the generation raised).
>   - *The operator* (`membership …`, run as a separate Unix user that owns the private
>     directory; the `-operator` name is recorded as given): `orders -attention` lists the
>     open matters with their age (also at the end of every `reconcile`), each closing by
>     itself when applied or settled, or by `attend -order … -reason …`; `grant -order …
>     -reason …` honours a payment that arrived but could not be granted (refused for a
>     removed licence, and says so); `reinstate -licence … -reason …` undoes a revocation made
>     by mistake (its end restored, the generation raised; the server activates its key again).
>   - *Receipts.* Claimed once in the transaction that makes the order paid ("owed"), mailed
>     after the answer, and retried by `reconcile` until sent.
> - **Questions 5 and 6 (§k) are answered:** a renewal paid before the end continues from the
>   old end at the locked price; after the end it is a new purchase at the list price from the
>   payment (the "During grace" and "In protection" lines of "Renewal" no longer apply).
This document says how the eight approved points are built, in small releases, without
locking any installed panel and without letting a licence state stop a hosted service
(D-022) or a security action. Where the owner has not decided something, it is listed as
open (§k), not chosen here.

## Main choices in one page

1. **Two levels instead of a lock.** The panel is either at **full** level or at the
   **protection** level. Every state that cannot authorise full use (licence ended past
   grace, rejected, unverifiable for 30 days, clock wrong, status unreadable; and, if the
   owner agrees, missing) gives the same protection level. Protection refuses only growth.
   The full-screen lock and its short exception list (`cmd/panel/license.go:192-212`)
   disappear.
2. **Growth is decided twice, defaults towards security.** A route table marks the growth
   routes, and the count check every creation of a domain, database or mailbox already
   passes (`cmd/panel/quota.go:105`) refuses in protection too. A new route nobody marked
   stays open (security never fails closed by omission); a new creation of a counted
   object is closed anyway (growth never fails open by omission). A route-inventory test
   fails when a route has no class.
3. **The signed file gets a second payload, not a replacement.** Today's panels refuse any
   format but `celikpanel-license-v1` and any unknown claim (`internal/licensing/license.go:175-190`),
   but they ignore unknown fields of the envelope around it (`license.go:273`, `:488-489`).
   The service therefore keeps signing v1 exactly as today and adds `payload_v2` /
   `signature_v2` beside it. Old panels keep working unchanged; new panels use v2; a
   rollback to an older panel reads the v1 part of the same file.
4. **Grace and protection are derived from dates**, not signed: grace ends 30 days after
   `expires_at`. v2 adds only what dates cannot give: a 30-day offline window, and two
   display facts (trial or not, automatic renewal or not) that never change a gate.
5. **The pair loses its licence dependency** by removing the licence check from the remote
   DNS machine endpoints and pairing endpoints on the receiving panel
   (`cmd/panel/middleware.go:31-35`, `cmd/panel/setup_dns_remote.go:288`,
   `cmd/panel/setup_dns_remote_admin.go:16`).
6. **A paid year starts when the payment is confirmed** (a renewal: from the old end if paid
   before grace ends). Not first activation: one clock with the payment provider's billing,
   refund window and receipts, and it is what the code already does (`Service.php:182`).
7. **The service stays the only issuer and signer.** A provider adapter turns "paid /
   refunded / cancelled" into order state; on every webhook the service asks the provider
   for the order's current state, so replays, duplicates and out-of-order events are
   harmless and a lost webhook is caught by a periodic reconciliation.

---

## Today, verified against the code

The inventory prepared for this work was checked where this design depends on it; the
differences found are in the appendix. The facts this design rests on:

- **The licence is already a one-year term.** The service sets `expires = anniversary(created)`
  at issue (`portal-membership/src/Service.php:182`, `:252-258`); the signed claims carry
  `expires_at` (`Service.php:285-288`); the panel judges `expired` at `now >= expires_at`
  (`internal/licensing/license.go:335-337`). Renewal is free and self-service in the last
  30 days or after expiry (`Service.php:259-269`); a late renewal starts a fresh year from
  the day it is made (`Service.php:265`).
- **The signed format is closed.** `Verify` requires `format == "celikpanel-license-v1"`,
  refuses unknown claims, and bounds `offline_until <= issued_at + 7 days` and
  `offline_until <= expires_at` (`license.go:175-190`).
- **The envelope is open.** The stored file and the service's answer are decoded with plain
  `json.Unmarshal` into `Envelope` (`license.go:273`, `:488-489`): unknown envelope fields
  are ignored, and `save` writes back only the fields the panel knows (`license.go:379-415`).
  Both are limited to 8 KiB (`license.go:263-272`, `:454-455`).
- **The service refuses unknown request fields** with 400 `invalid_request`
  (`portal-membership/http.php:45-46`); a panel would treat that as "unreachable", not as a
  rejection (`license.go:464-486`). This design therefore never changes the request.
- **States and the gate.** States come from `license.go:316-357`; the HTTP gate is
  `allowLicensedPanel` (`cmd/panel/license.go:279-301`) with the exception list
  `licenseRecoveryRequest` (`cmd/panel/license.go:192-212`). It runs for sessions and API
  tokens in `admitCallerGates` (`cmd/panel/middleware.go:255-260`), for remote DNS machine
  calls (`middleware.go:31-35` and again `setup_dns_remote.go:288`), for the pairing admin
  endpoints (`setup_dns_remote_admin.go:16`), and through `requireServerSetupAdmission`
  (`cmd/panel/server_setup_operations.go:2358-2367`) for setup start (`:1202`), the setup
  runner (`:1642-1652`), the publisher connection (`server_setup_dns_publisher.go:307`)
  and mail-enrollment continuation (`server_setup_mail_enrollment_continue.go:86`).
- **Nothing on the host consults the licence.** The Agent has no licence code; certificate
  renewal is the operating system's certbot (D-040); background jobs other than the setup
  runner never call the licence manager.
- **Explicit rejection codes.** `invalid_license`, `license_in_use`, `license_expired`,
  `invalid_activation` with HTTP 400 on refresh persist `rejected` (`license.go:465-480`).
  The error body is decoded into a struct with only `error` (`license.go:460-463`), so an
  extra field in the body is ignored by today's panels.

---

## a. Licence states and their timeline

### Timeline

```
 issued/paid                     T-30d     T-7d   T-1d   T = expires_at        T+30d
     |-------- active ------------|-- expiring (30/7/1) --|----- grace -----------|---- protection ---->
     full                          full                     full + notice           protection level

 last successful check C:  full while now < C + 30 days (v2) / C + 7 days (v1 only);
                           after that: unverifiable -> protection level until a check succeeds
```

The level is **full** only when all of these hold: a receipt for this server verifies, it is
not rejected, the clock is plausible, `now` is inside the verified window, and `now` is
before the grace end (`expires_at + 30 days`). Otherwise the level is **protection**.
Nothing else exists: no state stops a hosted service, and no state refuses a security
action, sign-in, one's own account, backups, updates or the licence screen.

### States

| State (code) | When | Level | Reason shown (administrator) | Who acts | Next action | How it continues |
|---|---|---|---|---|---|---|
| `trial` | v2 `term = trial`, more than 7 days left | full | Trial until {date} | Licence holder | Buy in the celikpanel.net account (optional now) | The same key becomes paid; the panel picks up the new date at its next check |
| `active` | Valid receipt, more than 30 days left | full | (no notice; licence screen shows the end date) | — | — | — |
| `expiring` | `expires_at - now <= 30 days`; tone rises at 7 and 1 day | full | Ends on {date}, {n} days left | Licence holder | Renew in the account (or nothing, if automatic renewal is on) | Next check (about hourly while used) or "Check again" |
| `grace` | `expires_at <= now < expires_at + 30 days` | full + notice | Ended on {date}; everything works until {grace end} | Licence holder | Renew before {grace end} | As above; a renewal in grace extends from the old end (§e) |
| `protection` | `now >= expires_at + 30 days` | protection | Ended on {date}; new domains, accounts and mailboxes are closed | Licence holder | Renew | As above; full level returns on the first successful check after renewal |
| `missing` | No `license.json` | protection (recommended; open question 4) | No licence on this server | Server administrator | Enter a key; trial without a card | Activation opens full level at once |
| `invalid` (rejected) | Service refused the refresh: moved, removed, key replaced, refunded | protection | celikpanel.net no longer accepts this server's licence ({reason} when known) | Licence holder | Open the account; move the licence back or enter another key | Activation or a successful check |
| `invalid` (file) | Bad signature, other machine, malformed file | protection | The licence file is not valid for this server (for example a copied server) | Server administrator | Check again; if it stays, enter the key again | As above |
| `clock_behind` | Clock more than 5 min before `issued_at` or `seen_at`, or set back while running | protection | This server's clock is behind (shows {server time}) | Server administrator | Turn on time synchronisation (command with Copy), then Check again | A check succeeds or the clock is right |
| `unreachable` | Last check failed, still inside the window | full | celikpanel.net cannot be reached; nothing to do; full use until {date} | Nobody now; in the last 7 days the server administrator | Allow outbound HTTPS to celikpanel.net, then Check again | The next successful check renews the window |
| `offline_ended` | Window passed without a successful check | protection | celikpanel.net not reached since {date} | Server administrator | Allow outbound HTTPS; Check again | First successful check restores full level |
| `status_unavailable` | `license.json` unreadable (not "missing"), manager not configured | protection | The licence could not be read just now; this does not mean it is missing or ended | Panel (retries); server administrator if it stays | Check again | The next readable status |

Precedence when several apply (the most actionable cause is shown): `invalid` >
`clock_behind` > `protection` > `offline_ended` > `status_unavailable` > `grace` > `unreachable`
notice > `expiring` > `trial` > `active`. `unreachable` and `expiring` may both show: the
date-based one first.

`expired` disappears as a state name: a receipt past `expires_at` is `grace` or
`protection`. The web bundle ships inside the same binary, so the browser and the server
change names in the same release; scripts that read the administrator API see the new
names, which the release notes state.

### Draft sentences

Administrator (EN / TR). Dates are the server's, shown in the reader's locale. "Your
account" means the celikpanel.net account holding the licence; where the administrator is
not that person the sentence adds "(or ask whoever holds this licence's celikpanel.net
account)" / "(ya da bu lisansın celikpanel.net hesabının sahibine söyleyin)".

| State | English | Türkçe |
|---|---|---|
| trial | This server runs on a trial licence until {date}. Everything works. To keep using it after that date, buy the licence in your celikpanel.net account; nothing changes on the server. | Bu sunucu {tarih} tarihine kadar deneme lisansıyla çalışıyor. Her şey çalışır. Bu tarihten sonra da kullanmak için lisansı celikpanel.net hesabınızdan satın alın; sunucuda hiçbir şey değişmez. |
| expiring (manual) | The licence for this server ends on {date} ({n} days). Renew it in your celikpanel.net account; the panel picks up the new date by itself. | Bu sunucunun lisansı {tarih} tarihinde bitiyor ({n} gün kaldı). celikpanel.net hesabınızdan yenileyin; panel yeni tarihi kendisi alır. |
| expiring, 1 day | The licence for this server ends tomorrow, {date}. Everything keeps working for 30 more days after that; renew in your celikpanel.net account. | Bu sunucunun lisansı yarın, {tarih} tarihinde bitiyor. Sonrasında 30 gün daha her şey çalışır; celikpanel.net hesabınızdan yenileyin. |
| expiring (automatic) | The licence renews automatically on {date}. Nothing to do; you can turn this off in your celikpanel.net account. | Lisans {tarih} tarihinde kendiliğinden yenilenecek. Yapmanız gereken bir şey yok; bunu celikpanel.net hesabınızdan kapatabilirsiniz. |
| grace | The licence ended on {date}. Everything keeps working until {grace end}. Renew in your celikpanel.net account before then; after that date new domains, accounts and mailboxes cannot be created, and everything else keeps working. | Lisans {tarih} tarihinde bitti. {ek süre sonu} tarihine kadar her şey çalışır. O tarihten önce celikpanel.net hesabınızdan yenileyin; sonrasında yeni alan adı, hesap ve posta kutusu oluşturulamaz, geri kalan her şey çalışmaya devam eder. |
| protection | The licence ended on {date}. Sites, mail, DNS, databases, backups, updates and security keep working. New domains, accounts and mailboxes cannot be created until the licence is renewed in your celikpanel.net account; the panel notices the renewal by itself within an hour, or at once with Check again. | Lisans {tarih} tarihinde bitti. Siteler, e-posta, DNS, veritabanları, yedekler, güncellemeler ve güvenlik çalışmaya devam ediyor. Lisans celikpanel.net hesabınızdan yenilenene kadar yeni alan adı, hesap ve posta kutusu oluşturulamaz; panel yenilemeyi bir saat içinde kendisi fark eder, "Tekrar kontrol et" ile hemen. |
| missing | This server has no licence yet. Everything already on it keeps working and you can manage its security and updates. To create domains, accounts and mailboxes, enter a licence key here; a 30-day trial at celikpanel.net needs no card. | Bu sunucuda henüz lisans yok. Üzerindeki her şey çalışır; güvenliğini ve güncellemelerini yönetebilirsiniz. Alan adı, hesap ve posta kutusu oluşturmak için buraya bir lisans anahtarı girin; celikpanel.net'te 30 günlük deneme kart istemez. |
| invalid (rejected) | celikpanel.net no longer accepts this server's licence: it was moved to another server, replaced, removed or refunded in your account. Everything keeps working; new domains, accounts and mailboxes cannot be created. Open your account to see the licence, then move it back here or enter another key. | celikpanel.net bu sunucunun lisansını artık kabul etmiyor: hesabınızda başka sunucuya taşınmış, değiştirilmiş, kaldırılmış ya da iadesi yapılmış. Her şey çalışmaya devam ediyor; yeni alan adı, hesap ve posta kutusu oluşturulamaz. Hesabınızda lisansa bakın, sonra lisansı bu sunucuya geri taşıyın ya da başka bir anahtar girin. |
| invalid (file) | The licence file on this server is not valid for this server, for example after the server was copied. Press Check again; if this stays, enter the licence key again. | Bu sunucudaki lisans dosyası bu sunucu için geçerli değil; örneğin sunucu kopyalandıysa böyle olur. "Tekrar kontrol et"e basın; düzelmezse lisans anahtarını yeniden girin. |
| clock_behind | This server's clock is behind (it shows {server time}). Until it is right, new domains, accounts and mailboxes cannot be created; everything else keeps working. Turn on time synchronisation with the command below, then press Check again. | Bu sunucunun saati geride (şu an {sunucu saati} gösteriyor). Saat düzelene kadar yeni alan adı, hesap ve posta kutusu oluşturulamaz; geri kalan her şey çalışır. Aşağıdaki komutla saat eşitlemeyi açın, sonra "Tekrar kontrol et"e basın. |
| unreachable | celikpanel.net cannot be reached right now. Nothing to do; the panel keeps working fully until {date}. | celikpanel.net'e şu an ulaşılamıyor. Yapmanız gereken bir şey yok; panel {tarih} tarihine kadar tam çalışır. |
| unreachable, last 7 days | celikpanel.net has not been reached since {date}. Full use continues until {date}; after that new domains, accounts and mailboxes wait for a successful check. Allow outbound HTTPS to celikpanel.net from this server, then press Check again. | celikpanel.net'e {tarih} tarihinden beri ulaşılamadı. Tam kullanım {tarih} tarihine kadar sürer; sonrasında yeni alan adı, hesap ve posta kutusu başarılı bir kontrolü bekler. Bu sunucudan celikpanel.net'e giden HTTPS bağlantısına izin verin, sonra "Tekrar kontrol et"e basın. |
| offline_ended | celikpanel.net has not been reached since {date}, so new domains, accounts and mailboxes cannot be created; everything else keeps working. Allow outbound HTTPS to celikpanel.net, then press Check again; full use returns on the first successful check. | celikpanel.net'e {tarih} tarihinden beri ulaşılamadığı için yeni alan adı, hesap ve posta kutusu oluşturulamıyor; geri kalan her şey çalışır. celikpanel.net'e giden HTTPS bağlantısına izin verin, sonra "Tekrar kontrol et"e basın; ilk başarılı kontrolde tam kullanım geri gelir. |
| status_unavailable | The panel could not read this server's licence just now. This does not mean it is missing or ended. Creating new domains, accounts and mailboxes waits; everything else keeps working. The panel tries again by itself; Check again tries now. | Panel bu sunucunun lisansını şu an okuyamadı. Bu, lisansın olmadığı ya da bittiği anlamına gelmez. Yeni alan adı, hesap ve posta kutusu oluşturma bekler; geri kalan her şey çalışır. Panel kendisi yeniden dener; "Tekrar kontrol et" hemen dener. |

The clock command is shown whole with a Copy button (`CopyCommand`), never inside the
sentence: `timedatectl set-ntp true`.

**Resellers and customers see nothing** in any state, because they cannot act on the
server's licence. Only when the protection level refuses one of their growth actions do
they see a reason beside the disabled button and in the refusal:

| Role | English | Türkçe |
|---|---|---|
| Customer | New {domains / mailboxes / databases} cannot be created on this server at the moment. Everything you already have keeps working. Contact your provider if you need one now. | Bu sunucuda şu an yeni {alan adı / posta kutusu / veritabanı} oluşturulamıyor. Mevcut her şeyiniz çalışmaya devam ediyor. Hemen gerekiyorsa sağlayıcınıza başvurun. |
| Reseller | New customers, domains and mailboxes cannot be created on this server at the moment. Your customers' existing services keep working. Contact the server administrator. | Bu sunucuda şu an yeni müşteri, alan adı ve posta kutusu oluşturulamıyor. Müşterilerinizin mevcut hizmetleri çalışmaya devam ediyor. Sunucu yöneticisine başvurun. |

The word "licence" is not used to them, and no date, state or provider detail is sent to
their browser (below).

### What the browser reads

`GET /api/v1/license/access` (`cmd/panel/license.go:218-277`) today returns
`can_use_panel`, `valid_until`, `valid_for_seconds`, `state`, `observation`, `reason` to every
role. It becomes:

- every role: `level` (`full` | `protection`), `can_use_panel` (true at both levels; false only
  if the access answer itself cannot be produced), `growth_open` (bool);
- administrators only: `state`, `reason`, `observation`, `term` and `renewal` (v2 facts),
  and times as seconds left on the server's clock, never absolute instants to compare
  (D-032): `term_ends_in_seconds`, `grace_ends_in_seconds`, `full_for_seconds`,
  `verified_seconds_ago`.

`LicenseOnboarding` stops redirecting to `/activate`; the License screen stays in Settings,
the Dashboard notice (`web/src/components/LicenseNotice.tsx`) carries the table above, and
growth buttons use the shared `BlockedReason` pattern (OPERATION-GUIDANCE "Blocking
problems"). `web/src/lib/licenseOffline.ts:23` (warning in the last 48 hours) becomes the
last 7 days of the 30-day window.

---

## b. The protection state's gate

### The rule

> In the protection level a request is refused only if it would add a new hosted object of
> one of these kinds: an **account** (customer, reseller, team member) or a product granted
> to an account; a **domain name** (domain, subdomain, alias, imported or copied site); a
> **mail address** (mailbox, forwarder); a **database or database user**; a **file-transfer
> login**; an **application** (catalogue install, service app, program).
> Everything else passes: reading, changing what exists, deleting, backups (making, restoring,
> downloading), exports, security, signing in and one's own account, updates, the licence
> screen, server administration and the pair.

It is enforced at two independent points:

1. **Route table** (before any work starts): the growth routes listed below, matched by
   method and path; domain subroutes by the `kind` that `matchDomainSubroute` already
   assigns (`cmd/panel/main.go:324-592`). A growth route is refused with 403
   `license_protection` (known state) or 503 `LICENSE_VERIFICATION_UNAVAILABLE` /
   `LICENSE_STATUS_UNAVAILABLE` (unknown state, D-024/D-025: unknown is not a verdict).
2. **Count check**: `checkSubscriptionQuota` (`cmd/panel/quota.go:105`) refuses the kinds
   `domains`, `databases` and `email accounts` with the same codes when the level is
   protection. Every creation of those objects already passes it (D-044) — for example
   `domain_handlers.go:441`, `domain_mail_handlers.go:279`, `domain_database_handlers.go:429`,
   `database_v2_handlers.go:639`, `app_catalog_handlers.go:400`, `site_copy.go:209`,
   `import_handlers.go:361`, `wordpress_tools_handlers.go:644`. The `disk` kind is never
   affected, so backups and restores (`domain_backup_handlers.go:202`, `:342`,
   `remote_backup_worker.go:707`) are untouched.

**Default for new routes.** A route that is not in the growth table is open: a forgotten
security action can never be refused for a licence reason. A forgotten creation of a counted
object is still refused by the count check. A route-inventory test (extending
`cmd/panel/license_test.go:73-141`) walks every registered route (`main.go:1613-1987`,
`account_recovery_handlers.go`, `recovery_access.go`) and every domain subroute kind with
its methods, and fails when one has no class in the table — so the default is a safety net,
not the design.

The licence check stays in `admitCallerGates` after the role and suspension gates
(`middleware.go:255-260`); it only needs the route class and the level. Background work
(Agent, schedulers, scheduled backups, certificate renewal) never consults the licence, as
today.

### Every route the lock refuses today, sorted

Today every authenticated route outside `licenseRecoveryRequest` is refused while locked.
Classes: **R** read, **A** sign-in and own account, **U** updates and licence, **S** security
and recovery, **B** backups and exports, **D** delete, **C** change what exists or server
administration, **P** pair (never licence-checked), **G** growth (closed in protection).
GET/HEAD of every route is **R** and is not repeated.

| Routes (registration) | Methods | Class |
|---|---|---|
| `/auth/logout`, `/auth/password`, `/auth/unimpersonate`, own e-mail (`main.go:1615`, `:1715-1716`, `:1726`), 2FA (`:1721-1725`), own and others' sessions (`:1620-1627`), guide state, activity log (`:1718-1720`) | all | A |
| Panel access address list (`:1628`) | PUT | S |
| Licence (`:1797`), update check/start/abandon/status/discovery (`:1798-1803`), version, host-mutation readiness, recovery status | all | U |
| `/api/v1/users` (`:1641`) | POST | **G** (new account) |
| `/api/v1/users/{id}` (`:1642`) | PUT, DELETE, "Log in as" | C, D |
| `/api/v1/team-members` (`:1643`) | POST | **G** |
| `/api/v1/team-members/{id}` (`:1644`) | PUT, DELETE | C, D |
| Plans, reseller plans, rights (`:1645-1650`) | all | C |
| Remote backups (`:1652`) | all | B |
| `/api/v1/subscriptions/{id}/entitlements` (`:1654`, `entitlements.go:207`) | POST | **G** (product granted) |
| same | DELETE | D |
| Subscription transfer, transfer requests (`:1655-1656`) | all | C |
| Suspension "deletable" (`:1658`) | DELETE | D |
| Security emergencies and notices (`:1661-1664`) | all | S |
| Usage notices, notifications, preferences, settings (`:1666-1674`) | all | C |
| Store catalogue admin, service-app and runtime-app offerings (`:1678-1685`) | all | C |
| Remote DNS pairing admin: enrollments, clients, connections (`:1688-1690`) | all | **P** |
| Remote DNS machine: accept, receiver status/publish, host record (`:1691-1694`) | all | **P** (no licence check at all, §d) |
| Server setup: plan, start, operation, revise, extend, complete, guidance, components, records, delegation, package updates, mail enrollment, publisher (`:1695-1714`) | all | C (server configuration, not a hosted object) |
| Setup join codes, peer, peer connect, peer retire (`:1701-1704`) | all | **P** |
| API tokens, remote API setting (`:1729-1731`) | all | A |
| Managed services scan, system check, metrics settings and "seen" (`:1734-1751`) | POST/PUT | C |
| PHP pools/config/extensions, nginx global/SSL/default HTTPS, ACME providers, nameservers, DNS cluster/setup, rate limits, directives (`:1757-1775`) | all | C |
| fail2ban jails, banned addresses, config (`:1778-1780`) | all | S |
| Server IP addresses, Postfix queue, Dovecot stats, `/config`, service action (`:1784-1792`) | all | C |
| Passwordless mail (`:1787`, D-046) | all | S |
| Component status/install/operation/candidate/uninstall/removal impact/preview/plan, mail profile, Roundcube update, PostgreSQL service, PHP tools (`:1804-1818`) | all | C |
| Firewall (`:1820`) | all | S |
| Repository, confine preview, vendor lane (`:1821-1823`) | all | C |
| Panel certificate, panel port (`:1825-1827`), security audit (`:1828`) | all | S |
| OS updates, origins, mode, summary (`:1830-1833`) | all | S |
| SSH password login (`:1835`) | all | S |
| File access tools, restart needs, clock, processes, reboot (`:1837-1843`) | all | C |
| Certificates overview and requests (`:1845-1847`) | all | S |
| OS release review, support table, offered versions (`:1848-1854`) | all | C |
| Database tool proxy, webmail entry and address (`:1855-1861`) | all | C |
| `/api/v1/domains/create` (`:1865`) | POST | **G** |
| Domain: delete, deletion status (`main.go:339-341`) | DELETE | D |
| Domain: hosting, app start/stop/restart/account/node-version/packages/env, PHP, PHP pool and settings, general, web settings, directives, cache clear, site config keep/take/recreate/apply, redirects, logs retention, Git, Git key, Git deploy, DNSSEC, DNS zone, DNS records, cron (`main.go:342-369`, `:372-373`, `:391-397`, `:402-404`, `:437-438`, `:533-542`, `:548-562`) | all | C |
| Domain: logs | DELETE | D |
| Domain: aliases (`:370-371`) | POST | **G** |
| Domain: alias item (`:567-568`) | DELETE | D |
| Domain: SSL mail, retry, renewal, Let's Encrypt, upload, settings, SSL (`:374-390`) | all | S |
| Domain: copy (`:398-401`) | POST | **G** |
| Domain: databases (`:407-408`) | POST | **G** |
| Domain: database item (`:576-580`) | DELETE | D |
| Domain: database users (`database_access_users.go:44-45`) | POST | **G** |
| Domain: database user, user password (`database_access_users.go:46-55`) | DELETE, POST | D, C |
| Domain: database tools — export, check (`matchDatabaseToolsRoute`) | all | B |
| Domain: database tools — import, import upload | all | C |
| Domain: database tools — copy (creates a database, `database_tools_handlers.go:596`) | POST | **G** |
| Domain: files, files download (`:409-412`) | all | C, B |
| Domain: backups, schedule, restore, download, contents, remote policy/send/restore (`:413-436`) | all | B |
| Domain: mail accounts (`:449-450`) | POST | **G** |
| Domain: mail accounts | PUT, DELETE; password, recovery, empty (`:441-448`) | C, D |
| Domain: mail forwardings (`:458-459`) | POST | **G** |
| same | DELETE | D |
| Domain: mail setup/auth/DKIM/apply/automatic/autoconfig/catch-all/autoresponder/options/outgoing (`:451-474`) | all | C |
| Domain: catalogue app install (`:475-476`) | POST | **G** |
| Domain: app operation retry/credentials/remove (`:569-573`) | POST, DELETE | C, D (a retry that would create a database is still refused by the count check; the operation stays recorded and resumes after renewal) |
| Domain: service-app install (`:485-486`) | POST | **G** |
| Domain: service-app update/control/secrets/uninstall (`:487-496`) | all | C, D |
| Domain: runtime-app create (`:501-502`) | POST | **G** |
| Domain: runtime-app settings/deploy/control/env/remove (`:503-514`) | all | C, D |
| Domain: file access (`:522-526`) | PUT | C |
| Domain: file access preview (`:527-528`) | POST | R |
| Domain: FTP/SFTP logins (`:529-532`) | POST | **G** |
| Domain: FTP/SFTP login item (`:574-575`) | PUT, DELETE | C, D |
| Domain: protected folders and their users (`:543-547`) | all | S |
| VPN status, setup, sync, peers (`:1886-1890`) | all | S |
| PowerDNS enable, server host name DNS, infrastructure zones, DNS engine, reconcile, switch (`:1891-1901`) | all | C |
| DNS peer inspector tool (`:1898`) | all | P |
| Mail configure/policy/outgoing/relay/test/server settings/sub-addressing/sender rules/report/notices/plan limits (`:1902-1914`) | all | C |
| WordPress tools (`:1917`) | all | C (a clone that creates a database is refused by the count check) |
| Malware scan (`:1918`) | all | S |
| System SQLite databases, Node.js runtimes (`:1926-1932`) | all | C |
| cPanel import inspect (`:1936`) | POST | R |
| cPanel import apply (`:1937`) | POST | **G** |
| `/api/v1/database-servers/{id}/databases`, `/users` (`main.go:762-770`) | POST | **G** |
| `/api/v1/database-servers/{id}/admin-account` (`main.go:774`) | all | S |
| Database grant forms, `/databases/{id}` grants, `/database-grants/` (`:1969-1983`) | all | C |
| `/databases/{id}`, `/database-users/{id}` | DELETE | D |
| `/database-users/{id}/password` | POST | C |
| Remote database access (`:1978`) | all | S (closing access must always work) |
| `/config/mysql`, `/config/php` | all | C |

Server setup is class C because it configures the server's own roles and creates no hosted
object for anyone; its runner stops waiting in phase `license`
(`server_setup_operations.go:1642-1652`). If the owner prefers setup to need a licence,
setup start and extend move to G; the rest of the table does not change.

### API tokens

A token is admitted through the same `admitCallerGates` (`cmd/panel/api_token_auth.go:668`)
and may do exactly what a session of the same role may do at the same level: at protection
level everything except growth. Growth answers 403 `license_protection` with the customer or
reseller sentence for those roles, so integrations can tell it apart from a permission
error and retry after renewal. Tokens never see licence dates or state (they get the role's
answer from `/license/access`).

---

## c. The signed licence file

### What the panel needs

| Fact | Today | Needed for | Source in this design |
|---|---|---|---|
| Term end | `expires_at` (v1) | expiring, grace, protection | v1 and v2, unchanged |
| Grace end | — | grace, protection | derived: `expires_at + 30 days` (constant in the panel and the service) |
| State | derived | everything | derived, never signed |
| Offline window up to 30 days | v1 allows at most 7 days and never past `expires_at` (`license.go:187-188`) | point 4; full use in grace while offline | **v2** `offline_until <= issued_at + 30 days`, `<= expires_at + 30 days` |
| Trial or not | — | the trial sentence | **v2** `term`: `trial` / `paid` / `free` (display only) |
| Automatic renewal | — | "renews automatically" instead of "renew" (D-024 who acts) | **v2** `renewal`: `manual` / `automatic` (display only) |
| Plan or price class | — | nothing in the panel (one edition, nothing counted) | not carried |
| Why a refresh was refused | error code only | "moved / removed / refunded" | an extra `reason` field in the 400 error body beside `error`; today's panels ignore it (`license.go:460-463`) |

v1 cannot carry the design unchanged: its 7-day bound and its cut at `expires_at` are
checked inside `Verify`, which every installed panel runs, and an extra claim breaks it.
Grace and protection *could* be derived from v1 alone; the 30-day window cannot.

### v2 and how both formats travel together

v2 claims (all required; decoder refuses unknown fields; payload at most 4096 bytes; same
Ed25519 key `PublicKeyHex`, `license.go:87`, the format string keeps the two apart):

```json
{"format":"celikpanel-license-v2","product":"celikpanel","license_id":"<32 hex>",
 "server_id":"<64 hex>","issued_at":0,"activated_at":0,"expires_at":0,
 "refresh_after":0,"offline_until":0,"term":"paid","renewal":"manual"}
```

Checks: v1's identity checks (product, `license_id`, `server_id` equal to this machine's,
`activation_token`), `activated_at > 0`, `issued_at >= activated_at`,
`expires_at > activated_at`, `issued_at < refresh_after <= issued_at + 86400`,
`0 < offline_until <= min(issued_at + 30 days, expires_at + 30 days)`, `term` and `renewal`
from their lists, and the 5-minute future check (`license.go:193-195`).

The service answers every `activate` and `refresh` with one envelope holding both:

```json
{"payload":"<v1, exactly as today>","signature":"…","activation_token":"…",
 "payload_v2":"<v2>","signature_v2":"…"}
```

- **Installed panels** decode the envelope with `json.Unmarshal` and ignore the two new
  fields (`license.go:488-489`), verify v1 as today and save only the fields they know.
  Their behaviour is exactly today's: D-039's 7 days, the lock at `expires_at`. The service
  keeps signing v1 by D-039's rule (`Service.php:281-288`), so no installed panel sees any
  change. v1 is kept for as long as any supported panel may read only v1 — in practice
  indefinitely; it costs a few hundred bytes.
- **New panels** verify v1 as today and v2 with the v2 checks; v2 is used only if its
  `license_id`, `server_id`, `issued_at`, `activated_at` and `expires_at` equal v1's.
  Otherwise v2 is ignored (logged once) and v1 rules apply. They store both in
  `license.json` (with `seen_at` and `rejected` as today, plus `rejected_reason`).
- **v1-only answer** (service not yet updated, or rolled back): the new panel derives grace
  and protection from `expires_at` and uses a 7-day window. When the signed v1
  `offline_until` equals `expires_at` (the old cut at term end), the new panel reads the
  window as `issued_at + 7 days`, because that cut only existed to end access at expiry,
  which grace now replaces. A pre-D-039 60-second receipt is honoured exactly.
- Sizes: two payloads, signatures and token are under 1.5 KiB, well inside the 8 KiB
  limits of answer and file.

### Rollback

| Situation | Result |
|---|---|
| New panel, new service | v2: 30-day window, grace, protection |
| New panel, old service | v1 with derived grace and protection, 7-day window |
| Old panel, new service | exactly today (v1); the new fields are ignored |
| Panel rolled back to an older release | it reads the v1 part of the same file and applies its old rules at once (for example a server in grace locks again as it would have); it rewrites the file without v2 at its next check; updating again restores v2 at the next check after that. No file is ever written in a form an older panel refuses |
| Service rolled back | new panels fall back to v1 rules at their next check; nothing locks that did not lock before D-047 |

### Clock

Unchanged from D-039 (`license.go:285-305`, `:341-344`, `:505-512`): 5-minute skew, the
service's `issued_at` as anchor, local `seen_at` high-water mark, wall-versus-monotonic
detection while running. Two consequences change: `clock_behind` now gives the protection
level instead of a lock (security work is never blocked by a wrong clock), and expiry and
grace are compared on the server's clock, which cannot be moved below `seen_at - 5 min`
without being detected.

### The 30-day offline window

`OfflineGrace` (`license.go:46`) becomes 30 days for v2 receipts and stays 7 days for
v1-only receipts. The refresh schedule (about hourly while used, jitter, 1→30 minute
backoff, 30 s after a lapse, 3-second wait, `license.go:47-67`, `:543-589`) is unchanged.
After a lapse into protection the panel keeps checking on use, so a renewal or a restored
connection is noticed within the hour, or at once with "Check again".

---

## d. The pair

**Where the dependency is.** A joined server publishes its domains' records to the main
DNS server through the remote DNS machine endpoints (`cmd/panel/remote_dns_auth.go:10-17`).
On the main server those requests pass `allowLicensedPanel` twice — in `requireAuth`
(`middleware.go:31-35`) and in `handleRemoteDNSMachine` (`setup_dns_remote.go:288`) — and the
pairing admin endpoints pass it in `handleRemoteDNSAdmin` (`setup_dns_remote_admin.go:16`).
When the main server's own licence is not active, the joined server's domain creation,
deletion (also the background deletion resume, `remote_dns_deletion_resume.go:512`),
mail-authentication records and import publication (`setup_dns_remote_origin.go:267-298`)
stop, and its setup waits with `server_setup_dns_publisher_peer_license`
(`server_setup_dns_publisher_refusal.go:97-98`). Zone transfer itself is native and unaffected.

**What replaces it.** Nothing licence-related. The machine endpoints are authorised by the
pairing credential (`requireRemoteDNSMachineAuth`, `remote_dns_auth.go:35-42`) and the
receiving server's own DNS authority checks (`setup_dns_remote.go:270-274`); those stay. The
licence call is removed from all three places, so:

- the main server never refuses a publication because of its own licence;
- the main server never checks the joined server's licence either: the joined server's
  growth is decided by the joined server's own level when its user creates the domain;
- pairing and revoking on the main server are class P (never licence-checked).

The joined server keeps mapping a `license_required` / `LICENSE_*_UNAVAILABLE` answer to the
peer wait, because a main server running an older release still sends it; the sentence
becomes "The main DNS server {host} runs an older CelikPanel that refuses connections while
its own licence is not active. Updating CelikPanel on {host} removes this wait; this server
asks again by itself."

Test: main server in every non-full state (`missing`, `invalid`, `protection`,
`offline_ended`, `clock_behind`, `status_unavailable`); the joined server creates, deletes,
imports and adds mail-authentication records, with no licence call on the path.

---

## e. The service (`portal-membership/`)

### Data model additions

Additive only, in the existing style (`CREATE TABLE IF NOT EXISTS`, `ALTER TABLE … ADD
COLUMN` inside `atomic`, `Service.php:27-63`). Times are UNIX seconds UTC; money is integer
minor units with an ISO currency code (USD only now).

| Table / column | Contents |
|---|---|
| `members` + `lang` (`tr`/`en`), `founder` (0/1), `trial_used_at` | mail language, founder flag, one trial per member |
| `licenses` + `term_kind` (`free`/`trial`/`paid`), `price_id`, `renewal` (`manual`/`automatic`), `revoked` (time), `revoked_reason` (`refunded`/`operator`), `term_started` | what the panel's v2 needs; the locked price; revocation distinct from the member's own `removed` |
| `prices` (id, label `list`/`founder`, currency, amount_minor, active_from, active_until) | 99 USD list; the founder row when its amount is decided |
| `orders` (id, member_id, license_id, kind `new`/`renewal`/`founder_grant`, price_id, amount_minor, currency, provider, provider_checkout_id, provider_order_id UNIQUE, status `created`/`pending`/`paid`/`refunded`/`failed`/`expired`, created, paid_at, refunded_at, term_from, term_to, fulfilled_at) | one row per purchase; `term_from`/`term_to` let a refund put the term back exactly |
| `subscriptions` (id, member_id, license_id, provider, provider_subscription_id UNIQUE, status `active`/`cancelled`/`past_due`, price_id, next_charge_at) | automatic renewal, only when the customer chose it |
| `payment_events` (id, provider, provider_event_id, UNIQUE(provider, provider_event_id), type, occurred_at, received_at, order_ref, body_sha256, processed_at, outcome) | idempotency and audit; never the raw body |
| `invoices` (order_id, issuer `provider`/`celikbros`, number, url, issued_at) | with a merchant of record the provider issues the customer's invoice; a Turkish direct provider later needs e-arşiv via an integrator — not designed until chosen |
| `refunds` (order_id, amount_minor, requested_by `member`/`operator`, reason, provider_refund_id, status, created) | the 30-day refund |
| `reminders` (license_id, kind, term_end, sent_at, UNIQUE(license_id, kind, term_end)) | each reminder once per term |
| `operator_events` (id, operator, action, target, reason, before_json, after_json, created) and `notes` (target, text, operator, created) | back-office audit |
| `events` | existing; new kinds `trial_started`, `paid`, `renewed`, `refunded`, `revoked`, `renewal_changed`, `founder_granted` |

### Provider-agnostic payment adapter

```
interface PaymentProvider {
  checkout(Order): url                       // hosted checkout; card data never reaches celikpanel.net
  verifyWebhook(headers, rawBody): EventRef  // signature + timestamp tolerance, else refuse
  fetchOrder(providerOrderId): OrderState    // authoritative state: paid / refunded / cancelled / failed, amounts, subscription id
  refund(providerOrderId, amount): providerRefundId
  cancelRenewal(providerSubscriptionId): void
}
```

- **Endpoint:** `POST /account/?action=webhook&provider=<id>`, outside sessions and CSRF,
  own rate limit, raw body read before decoding. Bad signature: 401, nothing stored.
- **Pull, do not trust the event body.** After verifying the signature the service stores
  `payment_events` (insert-or-ignore on `(provider, provider_event_id)`), then calls
  `fetchOrder` and applies the order's *current* state through one state machine inside
  `BEGIN IMMEDIATE` (`Service.php:77-82`). A replay or duplicate hits the unique key and does
  nothing; events arriving out of order do not matter because the fetched state is current;
  the answer is 200 once stored.
- **Fulfilment is idempotent:** `orders.fulfilled_at` is set in the same transaction that
  issues or extends the licence; a second "paid" for the same order changes nothing.
- **Late or lost webhook:** after checkout the account page says "Payment is being confirmed
  — usually within a minute; nothing to do" with a "Check now" that calls `fetchOrder` for
  that order (a read; it never starts a second payment). A reconciliation job (portal-host
  cron, every 15 minutes) fetches every order `pending` for more than 10 minutes, and once a
  day every order and subscription changed at the provider in the last 7 days. A restored
  database backup is brought forward the same way.
- **Fake provider** for tests and stage 3: same interface, checkout and webhooks served only
  on the loopback development origin (the guard already used for test mail,
  `http.php:26-29`, `:49`), signed with a test secret; it can replay, reorder, delay and drop
  events.

### Term start: when the paid year begins

The documents disagree: `LICENSE:15-16` says "from first activation"; the code
(`Service.php:182`), the terms (`terms.php:5`), the portal (`view.php:56`) and the panel
(`web/src/i18n/screens/settings/en.ts:41`) say "from creation".

**Proposal: a paid year starts when the payment is confirmed; a trial when it is created;
beta free licences keep their creation date.** Reasons: the payment provider bills,
renews and counts the 30-day refund from the payment date, so one clock avoids a renewal
charged on one anniversary for a licence ending on another; the code already counts from
creation, so only `LICENSE` changes wording; an activation-based start lets a bought key sit
unused indefinitely, which complicates refunds and the founder grant. A customer who buys
before installing loses the days in between; the 30-day refund covers a mistaken purchase.

### Renewal

- **Before expiry** (manual, in the last 30 days as today, or any time with automatic
  renewal): new end = the anniversary of the old end.
- **During grace** (up to 30 days late): new end = the anniversary of the old end. The
  server worked fully during grace, so no days are free and none are lost; such a renewal
  counts as uninterrupted for the price lock (open question 5).
- **In protection** (more than 30 days late): new end = the anniversary of the payment date;
  this is an interruption, so the then-current list price applies (unless the founder price
  is permanent across interruptions, open question 2).
- **Automatic renewal** exists only when the customer turned it on in the account. The
  provider's billing date is set to the licence end; the service sends its own reminder 7
  days before. A failed charge changes nothing on the licence: the dates lead into grace as
  for anyone; a successful retry during grace extends from the old end. Turning it off
  cancels the provider subscription and sets `renewal = manual`.

Today's rule `anniversary(max(now, old expiry))` (`Service.php:265`) changes only for the
grace case.

### Trial, refund, move

- **Trial (from 1.0):** "Start a 30-day trial" replaces "Create a free licence"
  (`view.php:56`); `term_kind = trial`, `expires = now + 30 days`, no card. One trial per
  member, and a server that already had a trial cannot start another (its `server_id` is
  already stored). Buying converts the same licence: same key, nothing to re-enter, the year
  starts at payment. What happens when a trial ends is open question 3.
- **Refund (30 days, unconditional):** the member presses "Refund" on the order in the
  account, or the operator does it. The service calls `refund`, and on the confirmed state:
  a first purchase revokes the licence (`revoked`, `revoked_reason = refunded`, binding
  cleared, **`generation` raised in the same statement**: see "Revocations raise the
  generation" below); the server's next refresh gets 400 `invalid_license` with `reason: refunded`,
  i.e. the protection level, never a stopped service. A refunded renewal puts `expires` back
  to the order's `term_from`; the dates then decide the state. Whether renewals are
  refundable is open question 6.
- **Move:** unchanged mechanism and free: release in the account (`Service.php:220-228`),
  then activate the same key on the new server; term and key stay. The account page offers it
  as one step ("Move to another server") that releases and shows the key. The old server
  learns of it at its next check and goes to protection. The existing brake of 5 releases per
  member per day (`Service.php:222`) stays as an abuse limit; the operator can lift it.
- **Activation of an ended licence:** today `activate` refuses `license_expired` as soon as
  the term has ended (`Service.php:314`). It changes to refuse only after the grace end, so a
  server reinstalled during grace gets its grace back instead of looking unlicensed. An
  installed panel activating during grace receives a receipt it judges `expired` — the same
  lock it reaches today with the refusal, so nothing changes for it.

### Operator back-office

Stage 3 builds it as a **command-line tool on the portal host** (`php operator.php …`),
run by the owner over SSH as the user that owns the private directory (mode 0700,
`portal-membership/README.md:3-6`). No new web surface on a shared PHP host. Commands:
`search` (e-mail, licence id or key suffix, server IP, host name), `show` (member, licences,
orders, events), `extend <licence> <days> --reason`, `refund <order> --reason`,
`revoke <licence> --reason`, `restore <licence> --reason`, `note <target> <text>`,
`founder-grant` (§f). Every command writes `operator_events` with before and after values;
operators never see licence keys (no `reveal`). A web back-office later would need separate
operator accounts, mandatory TOTP, an address allowlist in the private configuration and its
own session cookie; it is not part of these stages. `revoke` and `restore` follow the rule
below.

### Revocations raise the generation (a rule for every later change)

*Added 23 November 2026 after the third look at PR #361.* The Go member service answers a
panel's refresh with 503 ("database behind", the receipt is kept) instead of 400 when the
panel's activation token is ours for a state the database does not know (confirmed by the
owner on 23 November 2026: asked "Yeni hizmet, veritabanı lisanstan eskiyse 'geçersiz' yerine 'şu an kullanılamıyor' desin mi?", he answered "evet"). That is safe only
while this invariant holds, so every change to a licence keeps it:

- **A licence row is never deleted.** Removal, refund, operator revoke, account closing,
  erasure and any later end mark the row; they never `DELETE` it.
- **Every revocation raises `generation` in the same statement** that clears or changes the
  binding, marks the row removed or revoked, or replaces the key: release, remove, rotate, a
  reinstall elsewhere at the same address (today), and refund, operator `revoke`, account
  closing and any future one. A generation never falls.
- **A revoked state is checked before the database-behind answer.** A row marked removed
  (and, once the column exists, `revoked`) is never "behind", whatever its generation.
- **Only a first activation binds without a raise** (from no server to one).

Tests that hold the service to it: `TestEveryLicenceChangeRaisesTheGeneration` (each
mutating operation of `Service`; a new revocation path is added to its table) and
`TestNoSQLRevokesWithoutRaisingTheGeneration` (no `DELETE FROM licenses` and no binding,
removal, revocation or key change without the generation in any non-test Go string of the
module). The second reads SQL literals only: SQL assembled at run time, or an explicit
`generation=?` whose value is not higher, is caught only by the first, for the operations in
its table. The PHP service is not checked by them (it is the parity reference and changes
nothing).

### Reminder e-mails

Sent by the service (reliable whatever happens to the server's own mail) to the member's
address, in `members.lang` (set from the page language at registration, changeable in the
account), from a daily portal-host cron run; `reminders` makes each one exactly-once per term.

| Kind | When | Manual renewal | Automatic renewal |
|---|---|---|---|
| Expiry warning | 30, 7 and 1 day before `expires` | all three | 30 days: "renews on {date} for {price}"; 7 days: the required reminder; no 1-day mail |
| Charge failed | when the provider reports it | — | yes, with the grace end |
| Grace started | day of expiry | yes | yes if not renewed |
| Protection soon / started | 7 days before and on the grace end | yes | yes |
| Trial ending | 7 and 1 day before | trials only | — |
| Payment and refund confirmations | on the confirmed state | yes | yes |

The service sends no marketing mail at all. Every mail concerns a licence the member holds;
the member may turn off the 30-day warning per licence, not the 7-day automatic-renewal
reminder, failures or confirmations. Whether local rules (Turkish commercial-message rules,
EU) need anything more for these service messages is for the owner's adviser to confirm.

### Storage, backups, what is never stored

- **SQLite stays.** One host, a handful of writes per hour, WAL and `BEGIN IMMEDIATE` already
  in place (`Service.php:26`, `:77-82`); orders and webhooks do not change the scale. A move
  to a server database is not needed and would add a service to operate.
- **Backups:** nightly `VACUUM INTO` a dated copy, encrypted and copied off the portal host;
  30 daily and 12 monthly copies; a restore test into a staging directory each quarter; after
  a restore the reconciliation job pulls provider changes since the copy. The private
  `config.json` holds the licence-signing secret (`portal-membership/init.php:12-14`): it is
  backed up separately, offline, in two places. Losing it would leave every panel unable to
  receive a new receipt (they would fall to the protection level after the offline window)
  until a panel release with a new public key — never a stopped service, but serious.
- **Never stored:** card numbers, security codes or bank details (the provider's hosted
  checkout takes them; no card field is ever posted to celikpanel.net); full webhook bodies
  (only a digest and the fields above); provider API and webhook secrets outside the private
  configuration; licence keys in clear in logs (unchanged rule, `http.php:109-110`).

### Moving or switching the member service (draft, not yet rehearsed)

*Added 23 November 2026 after the second review of PR #361, extended the same day after the
third look. Not rehearsed on any host; it is proved on a test copy before it is used. It
applies to moving the service to another host and to switching between the PHP service and
the Go one, in either direction.*

**Why the order matters.** A panel that refreshes against a copy of the database that is
behind (a licence issued, bound or rotated on the old side after the copy) gets an answer
about a state the copy does not know. The Go service answers 503 when the panel's activation
token verifies under the signing key but the copy does not know that state, and the panel
keeps its receipt (for at most 7 days after its last successful check, then it shows
"verification unavailable"). The PHP service answers `invalid_activation` (400), and the
panel locks its management screens. So the old side stops changing the file before the copy
is taken, and never changes it again.

**An in-place switch needs no copy.** Switching between the PHP and the Go service on the
same host and the same `members.sqlite` (both take the same SQLite locks on the same file)
is only the selector change of step 5, with the maintenance of step 1 around it. Steps 2–4
are for a move to another host.

1. **A day ahead (a move only).** Lower the DNS TTL of the service's name, so caches let go
   quickly at the switch. Panels and resolvers that looked the name up earlier keep reaching
   the old host until their cached answer expires: that is why step 2 is never undone on the
   old host. On the new host: its TLS certificate for the service's name is valid before the
   switch (otherwise first activations fail as "unreachable"), and `members.sqlite`, its
   `-wal` and `-shm`, `sessions/` and `config.json` are owned by the one Unix user that runs
   the service, with the private directory at mode 0700.
2. **Maintenance first, for good on the old host.** The web server answers 503 for
   `/account/` on the old side (pages and panel API). Panels treat a 503 as "unreachable" and
   keep their receipts for the offline window; only a new server's first activation waits.
   After a move the old host **stays** in maintenance (or is shut down) and is never made
   writable again: two writable copies with the same key are a fork, which no answer can make
   right. It is decommissioned only after the watch period of step 7.
3. **A consistent copy.** Use SQLite's backup API on the stopped service (`sqlite3
   members.sqlite ".backup copy.sqlite"`, or Python's `sqlite3` `backup()` where the
   `sqlite3` command is missing). Never a plain file copy of a live WAL database, and never
   `members.sqlite` without its `-wal`. `config.json` (the signing key) goes by the owner's
   own offline path, never through the repository or a message.
4. **Compare on both sides.**
   - A checksum (`sha256sum`) of the backup file on the old host and of the file that arrived
     on the new host: equal, or copy again. With maintenance on, this is exact and covers
     everything, members included (a registration, an e-mail verification or a password
     reset writes no event, so the counts below cannot see them).
   - `membership check-db` (with `CELIKPANEL_MEMBERSHIP_PRIVATE` naming the private
     directory) on both copies: every line equal, above all "members: …" and "to compare two
     copies: events …, newest event at …, licence generations in total …", and "result:
     every live licence can be served" on the new side. It also prints whether the
     database-behind check is on (it should be, see step 8).
5. **Switch the selector.** The web server's rule for `/account/` (which program it runs, or
   which host the name points to) is changed, and maintenance ends **on the new side only**.
   The selector is the owner's web-server configuration; PR #361 builds none.
6. **Keep the old file unchanged.** The old private directory is kept as it is, read-only,
   for at least the panels' offline window plus one refresh interval (7 days plus the
   interval), because a panel that is behind only shows itself at its next refresh, and the
   old file is the only place the missing change can be read from (the log line names no
   licence).
7. **Watch.** On the new side, a log line "database behind the signing key" means a panel
   presented a state this copy does not know. Find the change in the old file (step 6) and
   go back (step 9), or, if the newer file is lost, see step 8.
8. **When the newer file is lost, or after a deliberate restore of an older file.**
   - Set `"database_is_authority": true` in the new side's private `config.json`. The
     database-behind answer is then off: every unknown or older state is rejected as the PHP
     service does, so the affected panels learn that their licence is not valid here instead
     of seeing "unreachable" for a week. `check-db` shows the setting. Set it only when this
     file is deliberately the authority (a restore that was chosen, or a newer file that is
     gone for good), never during an ordinary move, where the 503 protects panels from a copy
     that is merely late.
   - Tell the members whose servers were bound or moved after the file's time: they release
     the licence in their account and activate it again on their server. Panels cannot tell
     their administrators this (they see 503, or a rejection), so the owner must.
9. **Going back.** Maintenance on the new side first. If the new side changed the file
   (licences issued, servers bound), copy its file back the same way and compare (step 4)
   before pointing the selector back. Never point back to the older file: the PHP service
   would reject every panel bound on the new side, because it has no "database behind"
   answer.

   **Once the store has been used, there is no going back to the PHP service on that file.**
   The Go store ends a licence whose payment was reversed by marking it "revoked". The PHP
   service does not read that mark. On such a file it would still let the member renew that
   licence for free and then let a server activate it again: a licence that was taken back
   would work. So after the first order, going back means the Go service on the same file, or a copy of
   it, never the PHP service: `membership check-db -for-php` must pass before the PHP service
   may serve a file, and it fails once the store holds orders or revocations. Before the first order, going back to PHP is as described above.

---

## f. Existing free licences and beta participants

- **Until 1.0** licences stay free exactly as today: `issue` creates a one-year `free`
  licence, renewal is free. The payment system runs only against the fake provider and then
  the chosen provider's test mode; a private-configuration switch `charging` stays `off`.
- **Panels updated during the beta** apply grace and protection to free licences too — strictly
  more permissive than today's lock, so nobody loses access.
- **At 1.0** (stage 5) a one-time migration, run by the owner's operator tool on a copy first:
  for every licence that is a beta participant's (definition: open question 2; recommended "a
  licence activated on a server before the 1.0 date") and not removed: `term_kind = free`,
  `expires = max(expires, anniversary(1.0 date))` (the free first year), `price_id = founder`,
  an `orders` row `founder_grant` with amount 0 for the record, `members.founder = 1`. No key,
  binding or signed format changes; panels see the later end date at their next check.
- **The founder price** is a `prices` row with label `founder`, its amount entered when the
  owner decides it; each such licence keeps that `price_id` under the price-lock rule. Its
  scope (per licence or per member, kept across interruptions or not) is open question 2.
- **After 1.0** new licences start as trials; `issue` no longer creates free licences.

---

## g. Texts that must change

A list of locations, not the rewriting; they change in stage 5 unless noted.

| Location | What it says now |
|---|---|
| `LICENSE:14-16` | "free of charge for one calendar year from first activation" |
| `LICENSE:21-22` | expiration does not authorise stopping sites or mail (keep; add the protection state) |
| `portal-membership/terms.php:4-5` | "Product and free license", one year "from creation" |
| `portal-membership/terms.php:8` | renewal free; "no automatic charges are made" |
| `portal-membership/terms.php:9` | all users locked without an active licence; 7 days offline |
| `portal-membership/terms.php:11-12` | data list (add payment data held by the provider) |
| `portal-membership/view.php:29` | renewal window message |
| `portal-membership/view.php:34` | "extended by one year" (renewal rules) |
| `portal-membership/view.php:54`, `:56`, `:59` | "Currently free", "Create a free license", "Renew for free" |
| `portal-membership/view.php:69`, `:72`, `:73` | remove/release/rotate: "closes its management screens", "up to 7 days" (stage 2) |
| `portal-membership/view.php:74`, `:82`, `:86` | "Renewal is currently free", "Extend free for one year", "no payment details required" |
| `cmd/panel/license.go:88` | legacy CLI "Get your free annual license" |
| `cmd/panel/license.go:289-299` | lock messages (stage 1 and 2) |
| `web/src/i18n/screens/settings/en.ts:14-75`, `tr.ts:6-65` | `license.getKey` ("free"), `license.description` ("from creation"), `license.restricted`, `license.expiredHelp`, `license.tenantHelp`, `license.updateHelp`, offline and clock keys (stage 1 and 2) |
| `web/src/components/LicenseLockScreen.tsx`, `LicensePanel.tsx`, `LicenseNotice.tsx`, `RecoveryAccess.tsx:142-149`, `web/src/lib/licenseOffline.ts:23` | lock screen, licence page, Dashboard notice, 48-hour offline warning (stage 1 and 2) |
| `web/src/i18n/en.ts:109-115`, `tr.ts:22-27` (`recovery.license*`), setup keys `setup.guide.license*`, `setup.licenseWaiting`, `accessHold.license*`, `setup.v5.wait.server_setup_dns_publisher_peer_license` | recovery, setup and peer-wait copy (stage 1 and 2) |
| `cmd/panel/server_setup_operations.go:1202`, `:1645`, `:2385-2387`; `server_setup_dns_publisher.go:307`; `server_setup_mail_enrollment_continue.go:86`; `server_setup_dns_publisher_refusal.go:97-98` | setup and peer licence messages (stage 1 and 2) |
| `download-portal/index.html:61`, `:62`, `:87`, `:94` | "free annual license", "currently free … from creation" |
| `README.md:120` | "obtain a free annual server license" |
| `docs/MEMBERSHIP_LICENSING.md` (+ `.tr.md`) `:10-17`, `:46-48`, `:67-108` | approved model "free", access policy table, lock |
| `docs/guide/en/26-panel-settings.md:17`, `docs/guide/tr/26-panel-settings.md:17` | "shows a license screen instead of its pages", 7 days |
| `docs/guide/en/31-without-panel.md:13`, `docs/guide/tr/31-without-panel.md:13` | "waiting for its license" (add the protection state) |
| `docs/OPERATION-GUIDANCE.md:146-147` (+ `.tr.md`) | "must not purchase one … automatically" — stays true; add that automatic renewal is only the customer's choice in the account |
| `ROADMAP.md`, `ROADMAP.tr.md` | done in this change (two places) |

---

## h. Security and abuse

| Concern | Today | With this design |
|---|---|---|
| One active server per licence | `server_id` and public IP binding (`Service.php:302-323`); a second server gets `license_in_use` | unchanged |
| Key replacement | `rotate` invalidates the old activation (`Service.php:241-251`); old server locks at its next check | old server goes to protection at its next check |
| Moving | release + activate (`Service.php:220-228`) | unchanged, one-step in the account; old server to protection |
| Shared key | second activation refused | unchanged |
| A server that blocks celikpanel.net after its licence moved away | full use for up to 7 days | **up to 30 days**; alternating moves could keep two servers at full level with monthly moves. Accepted as the cost of point 4 (an outage of celikpanel.net must not close panels for a month). Limited by the release brake, detected by a service-side signal (the same licence released between the same two `server_id`s repeatedly), handled by the operator |
| Clock set back | detected (`seen_at`, monotonic); lock | detected; protection level, so nothing is gained |
| Old file restored and clock set back | root can defeat any local check (`docs/MEMBERSHIP_LICENSING.md:110-118`) | same limit, with a 30-day instead of a 7-day window |
| Patched binary | root owns the server | same; the licence is an agreement, not DRM |
| Refund cycles | — | buy, use, refund repeats gives what a trial gives; recorded and visible to the operator; any limit is the owner's decision |

**What an attacker gains by forcing each state:** `unreachable` — only a delay (at most 30
days) before a move, refund or removal takes effect; never a longer term or grace, because
those come from signed dates. `clock_behind`, `invalid`, `status_unavailable` — the
protection level, i.e. less than they had. Protection itself — nothing: it is the floor and
keeps everything running.

**Privacy.** The service learns what it learns today: member e-mail, a derived server
identity (salted hash of `/etc/machine-id`, `license.go:152-163`), the server's local name,
its public IP, activation and renewal times. A refresh writes nothing except the first IP
binding (`Service.php:333-336`), so check times are not kept in the database (the web
server's access log may hold them; its retention is a portal-host setting to state in the
terms). v2 sends nothing new. With payments the provider holds card and billing details; the
service keeps only the order fields above and, if the provider reports them, name and
country for the invoice record. Nothing about hosted sites, accounts or mail ever leaves the
server (`terms.php:12`).

---

## i. Tests and lab

**Provable without the real service or a real provider**

- `internal/licensing`: v1 unchanged; v2 verification and bounds; envelopes with v1 only,
  both, a v2 that disagrees with v1, a tampered v2; every state boundary with a fake clock
  (30/7/1 days, `expires_at`, grace end, 7- and 30-day windows, clock behind, rejection with
  and without `reason`); and an unchanged older licensing package (as D-039 did with the
  alpha.112 package) reading a file written by the new panel.
- `cmd/panel`: the route-inventory test (every route × method × role × state → class and
  answer); the count check in protection for each counted kind and never for `disk`; API
  tokens; the pair (§d) with the main server in each non-full state; security, backup,
  download, update and own-account routes in every state; the setup runner no longer waiting
  for a licence.
- Web: notices and sentences per state and role; resellers and customers see nothing until a
  growth refusal; `BlockedReason` on growth buttons; no redirect to `/activate`.
- `portal-membership` (`deploy/test-membership.sh`): dual signing and PHP↔Go interop for v2
  (`internal/licensing/interop_test.go`); order state machine with the fake provider —
  signature refusal, replay, duplicate, reorder, delay, loss and reconciliation; renewals
  before, during and after grace; trial rules; refunds of first purchases and renewals;
  reminders exactly once with a fake clock; operator commands and their audit; the 1.0
  migration on a copy.
- The existing guard that refuses any test request to the live service stays
  (`license.go:442-446`, `:661-683`).
- **Native lab with the acceptance fixture** (`internal/licensing/acceptance_fixture.go`):
  the fixture's local receipt gains chosen dates so a disposable pair can be put in grace,
  protection and offline-ended, and the lab can show that security actions, backups,
  updates and pair publication keep working while growth is refused. This would amend
  D-027's sentence "A run made with the fixture never counts as evidence about license
  behaviour" to "… about the live licence service"; it needs the owner's yes when stage 2's
  lab run is prepared. The release guard (`deploy/release-acceptance-license-guard.sh`) is
  unchanged.

**Not provable here**

What the live service runs and which key it signs with; real provider signatures, payouts,
tax documents and checkout behaviour (only its test mode, stage 4); e-mail deliverability; a
real multi-week outage on an installed server; the 1.0 migration on the live database
(only on a copy, by the owner).

---

## j. Staged plan

Each stage is a small release of one to three jobs, useful and safe on its own. Every PR
names the affected invariant and P0 item, the schema or format transition, recovery and
evidence (AGENTS.md); open items stay open.

### Stage 1 — security and the pair never blocked; expiry warnings (panel only)

1. While the licence is not active, the lock admits every route of classes A, U, S, B and
   P from §b (security, own account, backups and their download, updates, licence, pair);
   other routes stay locked as today. Remote DNS machine and pairing endpoints lose their
   licence check (§d).
2. Expiry warnings 30, 7 and 1 day before `expires_at` for administrators (Dashboard notice,
   licence page), derived from v1; nothing for resellers and customers.

- Transition: none (no format, no schema); `/license/access` gains administrator-only
  `term_ends_in_seconds`.
- Recovery: a rollback restores today's lock; nothing persisted changes.
- Evidence: route-inventory test for the admitted classes in every state and role; pair
  tests; web tests for warnings; lab: a pair with the fixture receipt expired (needs the
  fixture dates, or stays open).
- Affected: constitution §2 (recovery usable when the verifier fails), P0.2.
- Open after it: growth and other changes still locked when not active; no grace.

### Stage 2 — grace, protection, 30-day offline window (panel + small service change)

1. The two levels: grace and protection derived from dates; the growth table and the count
   check; the lock, `/activate` redirect and setup licence wait removed; new sentences.
2. v2: the service signs `payload_v2` beside v1 and accepts activation until the grace end;
   the panel reads both; 30-day window.
3. (If it fits) the error `reason` field and the `term` / `renewal` display.

- Transition: signed format v2 added beside v1; `license.json` gains optional `payload_v2`,
  `signature_v2`, `rejected_reason`; no database schema.
- Recovery: §c rollback table; the service deployment is the owner's step; until then panels
  run v1 rules with grace and protection.
- Evidence: §i licensing, panel, web and interop tests; old-package downgrade test; lab with
  fixture dates (with the owner's D-027 yes).
- Affected: access gate, persisted licence evidence, P0.2; D-039 amended.
- Open: real multi-week outage; live service deployment.

### Stage 3 — the service: orders, renewals, refunds, reminders, operator tool (fake provider)

1. Data model, order state machine, provider interface, fake provider, webhook endpoint,
   reconciliation.
2. Reminders and member language; trial (built, switched on at 1.0).
3. Operator command-line tool; backup procedure written and tried on a copy.

- Transition: additive SQLite schema; old rows valid as `free` licences.
- Recovery: a rollback of the membership release ignores the new tables (old code reads only
  its columns); `charging` stays `off`.
- Evidence: PHP service and HTTP tests (§i); interop unchanged.
- Open: real provider; deliverability; portal-host cron confirmed.

### Stage 4 — the real provider, once chosen

1. Adapter for the chosen provider; secrets in the private configuration only.
2. Checkout and account pages for buy, renew, automatic renewal, refund — in the
   provider's test mode during the beta.

- Transition: provider identifiers in existing columns.
- Recovery: `charging off` returns to free; reconciliation repairs a gap.
- Evidence: provider test-mode end to end; signature and replay tests with real samples.
- Open: production payouts and tax documents (accountant).

### Stage 5 — texts, terms, 1.0

1. `LICENSE`, terms (new version string, `Service.php:113`), portal pages, panel licence
   screen, README, guide and membership documents (§g).
2. The 1.0 migration for beta participants (§f); `charging on` with 1.0.

- Transition: terms version; the one-time migration (dry run on a copy first).
- Recovery: the migration only lengthens terms and sets a price id; a backup is taken before.
- Evidence: migration test on a copy; text review TR/EN.
- Open: anything the accountant or the owner's adviser adds.

---

## k. Open questions for the owner

1. **Payment provider.** *Answered in part on 23 November 2026:* Stripe is the first
   payment method (D-047 amendment). Still open: the legal entity and country of the Stripe
   account, and whether live charging goes through it, through a merchant of record with
   documented payouts to Turkey (Polar or Creem; Paddle once it confirms payouts) or, for
   domestic customers, a Turkish processor — after the accountant's answer on VAT, e-arşiv
   and selling through a foreign merchant of record. *Blocks:* live charging only.
2. **Founder price and who is a beta participant.** Amount; per licence or per member;
   whether "permanent" holds after an interruption. *Recommendation:* a participant is a
   member with a licence activated on a server before the 1.0 date; the founder price
   attaches to each such licence and stays for as long as it is renewed, interruptions
   included, since the owner said "permanent". *Blocks:* the 1.0 migration (stage 5).
3. **When a trial ends.** Straight to protection (with warnings 7 and 1 day before), or the
   same 30-day grace as a paid licence? *Recommendation:* straight to protection: grace
   exists so a paying customer's late payment never hurts them, and a 30-day grace after a
   30-day trial doubles the trial while protection already keeps every site running.
   *Blocks:* the trial in stage 3.
4. **A server that never had a licence.** Same protection level as a licence that ran out,
   or keep today's lock for `missing` (with security, updates and the licence screen open)?
   *Recommendation:* the same protection level — one floor is simpler, and a security
   action should never depend on why a licence is absent. *Blocks:* stage 1's scope for
   `missing`, stage 2.
5. **What "without interruption" means for the price lock.** *Answered on 23 November 2026*
   *(D-047): no grace; a renewal paid before the end is uninterrupted, after the end it is a new*
   *purchase at the list price.* The earlier recommendation, kept for the record: a renewal
   paid before the grace period ends is uninterrupted and extends from the old end; after
   grace it is an interruption. *Blocks:* renewal logic in stage 3.
6. **Refunds of renewals.** *Answered on 23 November 2026 (D-047): there are no refunds at*
   *all.* The earlier question, kept for the record: does the 30-day unconditional refund apply to every paid year,
   renewals (manual and automatic) included? *Recommendation:* yes — a forgotten automatic
   renewal is the most likely reason to ask. *Blocks:* refunds in stage 3.

Not decided and blocking nothing now: a multi-server discount; currencies other than USD
(prices are stored with a currency, so adding one is data, not design).

---

## Appendix: where the code differs from the inventory

The inventory was accurate on everything this design depends on. Additions and small
corrections found while checking:

1. Setup admission is checked in two more places than listed: the publisher connection
   (`cmd/panel/server_setup_dns_publisher.go:307`) and mail-enrollment continuation
   (`cmd/panel/server_setup_mail_enrollment_continue.go:86`), both through
   `requireServerSetupAdmission`.
2. The stale "one-minute rule" comment is at `server_setup_operations.go:2348` and its Turkish
   twin at `:2357` (the inventory said `:2347`); it is code, so it is left for stage 1.
3. `portal-membership/view.php:34` is the "extended by one year" notice, not "free" copy;
   the "free" copy is at `:56`, `:59`, `:74`, `:82`, `:86`.
4. Not in the inventory, and central to §c: the envelope (stored file and service answer) is
   decoded without `DisallowUnknownFields` (`license.go:273`, `:488-489`), and the error body
   is decoded into a struct with only `error` (`license.go:460-463`). This is what lets v2
   and a refusal `reason` travel beside v1 without touching installed panels.
5. Not in the inventory: the service refuses any extra request field with 400
   `invalid_request` (`portal-membership/http.php:45-46`), which a panel treats as
   "unreachable"; hence no request change in this design.
