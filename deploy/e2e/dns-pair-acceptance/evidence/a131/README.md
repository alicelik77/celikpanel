# alpha.131 lab check: remote database access follows a PostgreSQL port change — 2026-11-08

Lab only (archlinux WSL guest, QEMU cell `u1u`, Ubuntu 24.04, 192.0.3.200; listed client = labnet 192.0.3.1).
No real server, real CA, licence service or celikpanel.net was touched. Every product action went through the
panel API as the owner/customer would press it.

- Base: published `v0.1.0-alpha.130` tag commit `67d6131d5` (acceptance-licence dist), installed with the owner's
  `get.sh` from the lab portal; one-server setup (web + mail, BIND); PostgreSQL 18 from the PGDG lane via Components.
- Candidate: `fix/a131-dbremote-port` `ef2d0e900` built as lab label 131 (identity bump only, lab commit
  `99fee558b`), offered on the lab portal, update pressed once through the panel's update API.
- Customer `cust1` on plan Basic (all permissions, `database_remote` listed), server switch on, database
  `cust1_u1_test_rpg` / user `cust1_u1_test_rpg_user`, address 192.0.3.1 (permanent).

## Results

| # | Step | Result | Numbers / evidence |
|---|------|--------|--------------------|
| 1 | 130: cluster 18/main moved 5432 → 5433 with "Change port"; switch on, address added, PostgreSQL restarted once from Components | PASS (Boston state reproduced) | record v1 `postgresql:5433` (`listen`, `auth_delay`), auto.conf `listen_addresses='*'`, `shared_preload_libraries='auth_delay'`; nft accepts 192.0.3.1 on 5433; panel firewall opens 5433; `psql … port=5433 sslmode=require` → `ok TLSv1.3`; non-TLS → `no pg_hba.conf entry … no encryption` |
| 2 | 130: "Change port" 5433 → 5432 | FAIL reproduced (as on Boston) | record still `postgresql:5433` (v1, no data_dir); after the 5-minute loop nft holds only `tcp dport 5433 drop`, no 5432 accept; panel firewall has no 5432; card `OWNER_LISTEN`, state `owner_managed`, rule `waiting`; server view "listen_addresses = '*' … was set outside CelikPanel"; psql 5432 → timeout (also 6 min later) |
| 3 | Update 130 → lab 131 pressed once, nothing else touched | PASS | update `succeeded` in 99 s (schema 77, agent commit = panel commit); healed by itself 404 s after Update was pressed (Agent 20:21:33 "the PostgreSQL cluster recorded on port 5433 runs on port 5432 now; CelikPanel's record follows it"); record v2 `postgresql:5432` with `data_dir=/var/lib/postgresql/18/main`; nft `tcp dport 5432 ip saddr 192.0.3.1 accept` + `tcp dport 5432 drop`, no 5433 rule; pg_hba block unchanged (same hostssl line, `sha256=8bc7dd659809311f`); panel firewall opens 5432; card `open`, rule `active`, client port 5432, host `host1.u1.test`; postmaster start `20:07:45.942388` unchanged (no restart); psql 5432 TLS → `ok TLSv1.3`; non-TLS refused; 5433 → timeout |
| 4 | Last address removed (customer API), then added again | PASS | after removal: `listen_addresses` still `*`, auto.conf digest `d72f21677c0c6be3` unchanged, no `RESTART_NEEDED`, record keeps `listen`/`auth_delay`; nft only `tcp dport 5432 drop`; pg_hba block empty; panel firewall closes 5432; psql → timeout. Re-add: rule `active` at once, psql ok after 1 s, postmaster start unchanged |
| 5 | Port changed again on 131: 5432 → 5434 | PASS | record + nft on 5434 already at +0 s after the port change answered; panel firewall 5434 by +6 s (forced apply 20:23:44); no 5432/5433 rule left; psql 5434 TLS ok; card client port 5434. (The port change itself restarts the cluster, as before.) |

## Observations (not failures)

- The heal after the update came with the first apply after the panel's start window (~5 minutes after the
  restart), not at once; within what the release notes allow (3-minute start window + 5-minute loop).
- On 130 after the port change the pg_hba hostssl line stayed in the block while the record lost its addresses.
- With no address left the card's engine state stays `open` (engine prepared, listen kept); the rule list is empty.

## Logs

`A131-u1u.log` (all steps, raw snapshots), `setup130-u1.log` (cell, get.sh install, setup, PostgreSQL, customer),
`C2-update-u1u.log` (the update press), `builds.txt` (dist commits/sha256, portal manifests). Secrets: API answers
are passed through the lab redactor; credential values were checked against every log (none found).
