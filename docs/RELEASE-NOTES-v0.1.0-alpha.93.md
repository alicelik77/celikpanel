# v0.1.0-alpha.93 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.93.tr.md)

This is the release that follows v0.1.0-alpha.92. The number was chosen by the
release preparer under the owner's standing instruction of 2026-10-13. Written
on 2026-10-16 (UTC), on the branch `fix/alpha93-small-truths`, from `main` at
the alpha.92 release commit `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a`.

## What changed

Six small corrections from the owner's screenshots of both installed servers on
alpha.92 (2026-10-16), each decided by the owner the same day:

1. **Core Mail no longer asks for Repair when mail is verified.** A profile is
   verified when a larger verified profile that contains all of its components
   (Webmail or Spam-Protected Mail) proves the same reconciliation on the same
   scan. Component state alone still proves nothing; a running or failed Core
   Mail attempt keeps its own state.
2. **"Repair webmail" is an ordinary button**, drawn like Manage. It is amber
   only when the server reports a webmail problem.
3. **The Dashboard says verified mail plainly:** "Mail is installed and verified
   on this server."
4. **No mail component, no "Partially installed".** With only Nginx (or PHP-FPM)
   present, the Mail stacks card says "Not installed" in the neutral tone; the
   Webmail card on Components keeps its "Some components are already present…"
   note, muted. One mail component (Postfix, Dovecot, Rspamd, Roundcube) still
   makes a profile partial.
5. **Recent activity names mail profiles:** "prepared Spam-Protected Mail during
   setup" instead of "prepared protected-mail during setup".
6. **Counts are pluralised:** "1 item", "3 items" (Turkish: "1 öğe", "3 öğe").

## What you will see after updating to alpha.93

- Frankfurt: the Core Mail card is calm, with no amber Repair sentence; the
  Dashboard's Mail stacks card reads "Mail is installed and verified on this
  server."
- Boston: the Mail stacks card reads "Not installed", neutral.
- Both: "Repair webmail" beside Roundcube is a plain button; Recent activity
  shows profile names; Domains says "1 item".

## Limits

- No native record, by the owner's instruction of 2026-10-16 (quick releases).
  Go, component and mounted tests only; nothing measured on an installed server.
- Schema 44, no migration; recovery behaviour unchanged. The API change is a
  rule change only: a mail profile may now be `verified` with its own latest
  attempt `none`.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.

## Publication record

Not published.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
