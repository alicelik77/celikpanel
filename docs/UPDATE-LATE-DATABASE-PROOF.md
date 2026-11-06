# The update's late database proof: why it is not delegated to the running panel

Status: design note, 6 Nov 2026 (alpha.127 preparation). Nothing here is
implemented. The known limit "the update's last database check still copies
the live database" stays open.

## What runs today

`update.sh` (the target release's copy in its recovery runtime) calls
`verify_database_publication_if_required` → `recovery verify-update-database`
→ `panel-checker` (`verifyRecoveryDatabaseMigrationWith`) at three points:

1. after the isolated migration, with agent and panel stopped (phase
   `active`): the published file must equal the receipt byte for byte and
   have no sidecars;
2. after the controlled agent/panel starts, before `completion.pending` is
   removed (phase `completion`);
3. after the Certbot scheduler is restored (phase `scheduler`).

In 2 and 3 (the "late" proof) the panel is running. The checker binds the
admission, publication and receipt records, keeps the retired database
unchanged, checks that the live file is still the published inode with the
panel owner (`databaseMigrationStableFile`), and proves the completed schema
and migration history from a private WAL-aware copy of the live database
(`checkCompletedUpdateDatabaseWALAware`). A commit or checkpoint while it
copies is the typed `panelDatabaseChangedError`. Before alpha.125 that one
refusal failed the update on Boston ("database publication is unverified");
since alpha.125 it is read again as a whole new proof (at most 20 reads,
0.5 s apart), and every other refusal is final. `rollback.sh` has the same
late proof (`historicalSchema`).

## The proposal and the decision

The alpha.126 notes proposed asking the running panel for its verified
schema/publication state over a local root-only endpoint, with the copy
check as fallback. **Not implemented for alpha.127.** Reasons, against
ROADMAP's constitution, D-025 and docs/RESILIENCE-CONTRACT.md:

- **Invariant 5 (recovery survives the candidate's failure).** The late proof
  is the updater's independent check of the candidate's database before it
  removes `completion.pending`. A panel answering about its own database is
  the candidate reporting on itself: a wrong migration, a panel that opened a
  different file or a bug in the answer would all say "verified". It can be
  an observation (invariant 2), never the proof that replaces the copy.
- **Invariant 2 (evidence meaning).** "Panel did not answer" and "timed out"
  must fall back, never pass. With the fallback the copy check stays the
  authority, so nothing is removed: a second path with its own failure modes
  is added, and the race it was meant to fix still lives in the fallback.
- **Invariant 1 (no new general root API).** A new socket in the panel that a
  root updater trusts needs peer-credential checks, a lifecycle and its own
  recovery story; that is not a small-release change.
- **Version skew is not the obstacle.** The target release's `update.sh` and
  checker run the proof and the panel just started is the same release, so
  an update from an older release would work. The objection is the evidence,
  not compatibility.

## Direction for a later release (needs its own review and lab round)

Remove the race at its source and keep the checker as the authority:

- **Snapshot read instead of a file copy.** Read the schema and migration
  history inside one SQLite read transaction on the live database: WAL
  snapshot isolation means a concurrent commit cannot tear what is read.
  Conditions: open as the panel account (never root, so `-shm` ownership
  cannot change), read-only, no checkpoint on close
  (`SQLITE_DBCONFIG_NO_CKPT_ON_CLOSE`), never `immutable=1` or `nolock`, and
  bind the main file's identity (dev/inode/owner) before and after as today.
- **Or a narrower late proof.** Proof 1 already established the byte-exact
  published database with the services stopped. Whether 2 and 3 need the full
  schema proof again, or only identity continuity plus a migration-history
  read, is a question for the contract review, not for this note.
- Keep alpha.125's bounded re-read until the replacement has native evidence.

Acceptance before any change: native updates 125→127 and 126→127 with real
panel start-up writes; an injected concurrent writer during the late proof;
the panel stopped or hung (no pass); a rollback across the change; the
existing `deploy/test-isolated-database-migration.sh`,
`test-recovery-runtime-shell-contract.sh` and
`test-update-panel-start-readiness.sh` contracts.

## Türkçe özet

Güncellemenin son veritabanı denetimini çalışan panele sordurmak alpha.127'de
yapılmadı. Denetim, adayın (yeni panelin) veritabanını bağımsız olarak
kanıtlar; panelin kendi hakkında verdiği cevap kanıt değil gözlemdir
(D-025 ilke 5 ve 2). Cevap gelmezse eski kopya denetimine dönmek gerekeceği
için yarış ortadan kalkmaz, yalnız ikinci bir yol eklenir. Daha sonraki bir
sürüm için yön: canlı veritabanını panel hesabıyla, tek bir okuma işleminde
(WAL anlık görüntüsü) okumak ya da durdurulmuş hizmetlerle yapılan ilk
kanıttan sonra son denetimi daraltmak. alpha.125'teki sınırlı yeniden okuma
o zamana kadar kalır; bilinen sınır açık kalır.
