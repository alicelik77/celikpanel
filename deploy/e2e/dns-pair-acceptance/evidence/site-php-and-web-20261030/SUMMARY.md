# Site PHP switch and web hardening — native check (2026-10-30)

Branch `fix/site-php-and-web` (fa9f0a74d..2bd30fc11), merged into
`release/alpha113`. Build e62c9897e (acceptance licence, `dist.json`), lab
QEMU/KVM guests, Pebble CA, installed through `get.sh` as in the final
acceptance (`get.sh.diff`: only the pinned key digest). Debian 13 and Arch
standalone (PowerDNS, web, mail, webmail); setup 18/18 on both. Secret scan
clean (`secret-scan.json`); logs `setup.out`, `check.out` and the tarball.

| Check | Debian 13 | Arch |
|---|---|---|
| Certificate with the server clock 3 s behind the CA | waited ("valid from …, 2s after this server's time"), active in 6 s | same |
| HTTPS redirect after SSL at creation | on; http → https 301 (site and www) | same |
| https | 200, chain verifies | same |
| `Server:` header | `nginx` | `nginx` (was `nginx/1.30.6`) |
| Static → PHP | 200, PHP 8.4, pool written, socket recorded | 200, PHP 8.5 |
| PHP page | served as the site user | same |
| Back to static | 200; `.php` → 403 | same |
| Webmail address | http 301 → https 200 | same |
| `nginx -t` | ok | ok |

Commit 34841b503 changes only refusal wording (component tests). Seen here
and fixed afterwards in 132e895cf: Arch's PHP sent `X-Powered-By:
PHP/8.5.12` (`expose_php = On`); nginx now hides that header. The
"owner_configured" renewal state seen in the lab is the lab's own certbot
`cli.ini` pointing at Pebble, not a product defect.
