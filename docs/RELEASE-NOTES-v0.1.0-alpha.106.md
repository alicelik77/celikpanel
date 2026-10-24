# v0.1.0-alpha.106

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.106.tr.md)

This is the candidate that follows v0.1.0-alpha.105. **It is not published, and
nothing from it is on an installed server.** Written on 2026-10-24 on the branch
`fix/alpha106-clear-removal`, from `main` after the publication record of
alpha.105 (the alpha.105 release commit is
`0a51195cd294be5f009f2a2b72e1dd372094e524`).

## Who should update, and why

Anyone who removes components. On Frankfurt the owner pressed the remove button on
the PHP-FPM row (8.6, 8.5 and 8.4 installed) and asked: "if I say remove, which
version will it remove?" The dialog did not say it in its title, and its refusal
was buried in the middle (D-037 amendment, 2026-10-24).

Only the owner of a server updates it, from the panel's own update screen. What the
owner sees is in the [operation guidance](OPERATION-GUIDANCE.md#removing-a-component-or-a-version-which-version-and-why-it-cannot-go-now-d-037-amendment-2026-10-24).

## Which version

- When PHP-FPM has several versions, its remove button first asks **"Which PHP-FPM
  version do you want to remove?"**: one choice per version with who uses it
  ("Used by phpPgAdmin, Roundcube"), and **All versions (8.6, 8.5, 8.4)** set apart
  below. Nothing changes there.
- A version continues into that version's own removal (with "move them, then
  remove" when only tools are in the way); All versions into the whole one.
- With one version there is no question; the title names it ("Remove PHP-FPM
  8.4?"). Every title says what goes ("Remove PHP-FPM — all versions (8.6, 8.5,
  8.4)?").

## The dialog leads with the answer

- First: **"PHP-FPM can't be removed now"** (or "This removes …" when it can), then
  each cause once, in plain words.
- Then **What you can do**, with the way to do it: "Remove phpPgAdmin first" opens
  that removal; sites go to Domains; "Or keep PHP-FPM: close this window and nothing
  changes."
- The package list, what apt or pacman does and the package manager's own
  explanation are under **Details**. A refused removal is never said to "take"
  packages.
- A refused removal offers **Close**; the danger button reads **"Can't remove"**
  and is visibly unavailable. This now also holds for a component that still hosts
  sites or databases (the Panel refused those before too).

## Limits

- Node.js versions are removed from their versions list only; PostgreSQL has no
  per-version removal in the Panel.
- Web only: no Agent or Panel change; sequence 106, schema 44 unchanged.
- Not measured on an installed server. Web 1805 tests passed and the production
  build is within its budgets; no Go package changed.
