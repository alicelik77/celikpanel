# User guide: the content format

*alpha.119 · Kullanım kılavuzunun içerik biçimi; Türkçe bölümler de bu kurallara uyar.*

The guide is written once, as Markdown files, and built into two places:

- the panel (`web/src/help/guide/gen/`): a small index per role and language (chapter
  list, topics, search) and one lazy chunk per chapter and language, shared by every
  role that reads it, opened from the `?` in the page header and from `/help`;
- celikpanel.net: one page per chapter (`download-portal/guide/`).

`web/scripts/build-guide.mjs` reads the files, refuses anything outside this format,
and writes both. Run `npm run guide` in `web/` after every change and commit what it
wrote. `npm run build` and `npm test` fail while the generated files are stale.

## Files

- `docs/guide/en/NN-slug.md` and `docs/guide/tr/NN-slug.md`, the same file name in
  both languages. Every chapter exists in both, or the build fails.
- `NN` is two digits and sets the order. `slug` is lowercase `a-z`, `0-9` and `-`,
  in English in both languages, and equals the chapter's `id`.
- UTF-8, LF line endings, no byte-order mark.

## Front matter

Every file starts with exactly these five fields between two `---` lines:

```
---
id: files-access
title: Files: file manager, FTP, SFTP and SSH
roles: [customer]
topics: [domain.files#file-manager, domain.access#ftp-sftp]
keywords: [ftp, sftp, ssh, filezilla, upload, key]
---
```

| Field | Rule | Same in EN and TR |
|---|---|---|
| `id` | The slug of the file name. | yes |
| `title` | Plain text, at most 80 characters, no Markdown. | no |
| `roles` | One or more of `admin`, `reseller`, `customer`. A team member (additional user) reads the customer chapters. Roles choose what is shown first; they are not access control, and every chapter is public. | yes |
| `topics` | The screens this chapter explains: `topic` or `topic#anchor`. May be empty: `[]`. A topic belongs to one chapter only. | yes |
| `keywords` | Extra search words, in the file's language. May be empty. | no |

Topic names are the panel's screen anchors. The `?` on a page opens the chapter that
claims the page's topic, at its anchor. These topics exist today (from
`web/src/help/guide/routes.ts`); a page whose topic no chapter claims opens the first
chapter of the reader's guide and says that the page has no chapter yet:

`dashboard`, `domains`, `domain.overview`, `domain.hosting`, `domain.dns`,
`domain.mail`, `domain.databases`, `domain.apps`, `domain.ai`, `domain.files`,
`domain.access`, `domain.advanced`, `dns.infrastructure`, `databases`, `users`,
`team`, `store`, `vpn`, `components`, `server-setup`, `dns.servers`, `monitoring`,
`security`, `maintenance`, `import`, `audit`, `settings`, `account`, `help`.

`/users` is two pages: account management for the administrator and a reseller
(`users`), and the customer's team members for a customer (`team`).

A new topic name is added to `routes.ts` (for a page) or used in
`<GuideLink topic="…">` (inside a page). The build fails when a `GuideLink` names a
topic no chapter claims.

## Body

Only this subset of Markdown is accepted. Anything else is refused with the file and
line, so nothing is silently dropped.

| Write | For |
|---|---|
| `## Heading {#anchor}` and `### Heading {#anchor}` | Sections. Every heading has an anchor: `a-z`, `0-9`, `-`, unique in the chapter. No `#` or `####`; the title is the chapter's only top heading. |
| Lines separated by a blank line | Paragraphs. |
| `- item` or `1. item` | One level of list. Continue an item on the next line indented by two spaces. |
| `\| a \| b \|` with a `\|---\|---\|` line under the header | Tables. Every row has the header's number of cells. |
| ```` ``` ```` … ```` ``` ```` | Code: commands, file paths, output. Shown exactly. An optional word after the opening fence names the language. |
| `:::note` … `:::` and `:::warning` … `:::` | A note, or a warning about losing data or access. Paragraphs, lists and code inside; no headings, tables or nested blocks. |
| `**words**` | The exact name of something on screen: **Review the change**. |
| `` `text` `` | A command, a file name, a value to type. |
| `[text](guide:id)`, `[text](guide:id#anchor)`, `[text](#anchor)` | A link to a chapter, or to a section of one. |
| `[text](https://…)` | An outside page. Only `https`. |
| `\*` `` \` `` `\[` `\]` `\|` `\\` | The character itself. |

Refused: raw HTML or anything that looks like a tag (`<b>`, `<br>`, `<!--`), HTML
entities (`&amp;`), images, block quotes (`>`), nested lists, `#` and `####`
headings, underlined headings, horizontal rules, reference links, an unclosed `**`
or `` ` ``, links to anything but `guide:`, `#` or `https:`, and a `guide:` link whose
chapter or anchor does not exist. `_` and a single `*` are not markup: they stay text.

A link to a chapter that the reader's role does not have is shown as plain text in
the panel and as a link on celikpanel.net.

## EN and TR must match

The build compares the two languages and fails when they differ in: the set of files,
`id`, `roles`, `topics`, the headings' anchors and levels (in order), and the set of
`guide:` link targets. Words, keywords, sentence order and examples may differ.

## Writing

- Say what the reader sees and what they press, with the names on screen in bold.
- For every state, follow the panel's own rule: the reason, who acts, the next action,
  how it resumes (see `reading-states`).
- Plain words. No internal codes, no decision numbers, no team words.
- Turkish is written as Turkish, not translated sentence by sentence.
- Never show a real password, key or address; use `example.com`, `203.0.113.10`.

## Size

Each chapter is its own chunk per language, at most 12 KiB raw and 4 KiB gzip; a
longer chapter is split into two. Each role's index holds the titles, headings,
topics and keywords of its chapters, at most 40 KiB raw and 14 KiB gzip; search
reads the chapters' text from the chapters themselves, so keywords matter for the
first results (`web/scripts/check-bundle-budget.mjs`; `npm run build` reports the
headroom).
