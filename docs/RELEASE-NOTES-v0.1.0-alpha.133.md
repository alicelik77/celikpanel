# v0.1.0-alpha.133

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.133.tr.md)

The second small design release. The left menu's **Databases** page now
lists everyone's databases by the one visibility rule, and what belongs to
the server itself has left it for Security and the MariaDB and PostgreSQL
pages. Fields are as wide as what they hold, deleting a domain says what
goes and what stays, a domain deletion that stayed "waiting" on a two-server
DNS pair finishes again, and a false blocklist alarm is gone.
**Only the owner updates installed panels, from the panel's own update
screen.**

## What changes for you

### Databases: one list, by who may see what

- **An administrator sees every database on the server,** a reseller their
  own customers', a customer their own. Before, an administrator saw an
  empty list ("No databases yet") although customers had databases.
- Administrators and resellers read one list with **Database, Owner,
  Domain and Type**. Under the owner it says "you", or which reseller a
  customer belongs to. One search looks in the name, the owner and the
  domain; a click on a column heading sorts by it. On a phone each row is a
  card. A customer sees their databases grouped by domain.
- **Each row says its actions in words:** "Open" goes to the domain's
  Databases section; the "More" menu has **New password…** (asks which user
  when there are several) and **Delete…** (lists what is lost and what
  stays and asks for the database's name to be typed).
- **Deleting a database works the same from this list and from the
  domain's Databases section, and leaves nothing behind:** the database
  goes, and with it every user that no other database uses. A user that
  another database also uses stays, and so do the database server's and
  CelikPanel's own accounts; both delete questions say which users go and
  which stay. A user its owner gave other rights on the server by hand
  (outside CelikPanel) stays on the server too, with only its access to the
  deleted database taken away, and the notice after the delete says so.
  On a database server CelikPanel reaches with that server's own account
  (another machine, or a database server an administrator registered
  themselves), only the database is deleted and its users stay; the
  question says so before, and the notice after.
  Before, deleting a customer's database from this list could be refused
  with "the server refused the credential CelikPanel holds", in English on
  a Turkish screen, and its users were left behind.
- A PostgreSQL database that a site is still connected to is deleted: its
  open connections are ended first.
- If a delete is interrupted, the database stays in the list and pressing
  **Delete** again finishes it; the screen says what happened (deleted, not
  deleted, or not known) and the audit log records every delete, also one
  that finished only partly.
- **"Add database"** asks "For which domain?" and opens that domain's
  Databases section with its form already open.
- Passwords and the contents of a database are never shown on this page.
  The list reads itself again after every change and every half minute.
- If the server could not be asked which database services run, the
  administrator is told exactly that, never "no database service".

### Server matters moved to their own places

- **"Customers may connect to their databases from outside"** is now on
  **Security**, in the card **Open to the outside**. It is saved only with
  Save, and the question before saving says what turning it on or off
  means. Under it each installed database service says "Ready · port ·
  encrypted connections only" or "Not ready", with why, who acts, the next
  step and one button, in plain words.
- **The account CelikPanel connects to MariaDB or PostgreSQL with** is on
  that service's page in Components, with Show password, New password… and
  Remove account… in its "More" menu.
- The administrator's Databases list ends with one quiet link to the
  outside-connection setting on Security. A domain's outside-connection
  card sends the administrator there instead of turning the switch on
  itself.
- The MariaDB page no longer suggests `bind-address = 0.0.0.0` for remote
  connections: that opens the server to every network.

### Fields sized to what they hold

Every text, number and date field and every list across the panel has one
of six sizes chosen by what it holds: a port or a count is short, an
address or a name medium, a host name, e-mail address or password wider,
a web address, path or command widest. On a phone a field never runs past
the screen. A test keeps it that way, with no exceptions.

### A domain deletion no longer stays "waiting" on a DNS pair

- On the main server of a two-server DNS pair, deleting a domain could stay
  "being deleted" for ever. To prove that the backup DNS server removed the
  zone too, the server ran its own full DNS check twelve times in one
  attempt, inside tight time limits; on a busy server the attempt always
  ran out of time, and after an earlier deletion it also had to clean up
  that deletion's one-time check first. The full check now runs four
  times, only where the proof depends on it; the writes in between check
  only what authorizes them (both locks, the exact attempt, the pairing).
  The earlier deletion's clean-up runs once. A deletion is expected to
  take about 9–12 seconds instead of 20–27 (TO CONFIRM after the lab run).
  These changes are proven by component tests; the real two-server check
  is the lab re-run that is still to come.
- When the backup's confirmation is late, the window says the truth: the
  domain is already removed from DNS on this server, only the backup's
  confirmation is open, **nobody needs to act** and CelikPanel checks again
  by itself (at most every 5 minutes). It shows as waiting, not failed, and
  finished steps read as done ("Mail removed", …). It also says which side
  ran out of time: this server's own check or the backup's answer.
- The message follows the state: it continues by itself; it is stuck and
  the administrators were notified; or the automatic checks have ended. If
  the same cause repeats six times in a row (about a quarter of an hour),
  the administrators get **one** notification with the reason, who acts and
  the next step; the re-checks continue, and More → Delete… on the
  domain's row is a real retry again. A re-check the panel stopped waiting
  for is not counted as a repeat, so no false "stuck" is reported.
- **A check record left half done is finished by itself.** To confirm the
  backup, the server keeps a one-time check record. alpha.132 could stop
  in the middle of retiring the previous deletion's record (the lab left an
  empty record file beside the old one), and every later check of the next
  deletion refused it, so that deletion never finished while the screen
  said nobody needed to act. The next check now finishes or undoes such an
  interrupted change by itself and carries on in the same attempt; a
  retirement is now one step that cannot stop halfway. What CelikPanel
  cannot attribute to such an interruption is kept untouched and said as
  its own state: the server administrator is named, with the one next step
  (send two command outputs to support; do not delete the files), and the
  administrators get one notification.
- **The waiting message is short and says who acts.** Domains shows the
  waiting deletion as one block: what waits, why in one sentence, who acts
  ("You do not need to do anything" only while CelikPanel's own checks can
  finish it), the next check's time, and the page's own "Check deletion
  status" button; the command for an administrator and the technical
  explanation are under a closed "Details". The deletion window and the
  notification use the same sentences and follow the same state (whether
  CelikPanel re-checks by itself, the administrators were notified, or a
  person must act); they name only controls the reader has, and the window
  points to the banner's Details for the commands.

### Other corrections

- **Deleting a domain** lists what goes (the website and its files, extra
  FTP logins, mailboxes and forwarding, CelikPanel's DNS records,
  certificates, the backup schedule) and what stays (databases stay on the
  database server but leave the panel; backups already taken stay but
  leave the panel; stored mail is moved aside; records at another DNS
  provider are untouched), and asks for the domain's name to be typed.
- **Plan limits:** a value outside its range is named beside Save, which
  stays off until it is corrected (Save used to stay on and the server
  refused). The notice levels (50–95 %) keep what is typed. A value above a
  key's own maximum names that maximum, never "at most no limit".
- **"Send through another server" (Postfix):** the last form with its own
  Save now saves like every other screen, with marked changes, the save bar
  and the leave question. "Test the connection" tries the typed values,
  saves and sends nothing, and says where it stopped: not reachable, not a
  mail server, no encrypted connection, or user name or password refused.
- **Blocklist check:** the panel no longer reports a listing when a list
  refuses the question. Spamhaus does not answer questions that come
  through a public DNS resolver, and that refusal was read as "listed".
  Each list's answer is now read against its documented codes: listed, not
  listed, or **could not check** with the reason; a list that could not be
  asked is a grey line that never colours the mail card or the domain. A
  policy entry (Spamhaus PBL) is said as one. dnsbl.sorbs.net, shut down in
  2024, is no longer asked.
- **A program or a service app is easy to find:** in a domain's Site
  section the part that is the site ("Programs", or "AI and automation")
  comes first and open, and the Overview's Site tile says what runs there
  and goes straight to it.
- Turkish uses "Parola" for a password and "Tekrar dene" for retry
  everywhere; the domain list's title comes before the plan cards on a
  phone; the notice's close button is named in your language.
- **The update window follows the update to its end** even when the page
  was held for a moment during the update (it used to stay on its second
  step when this browser had done an earlier successful update). An update
  press refused before anything changed leaves no failed service unit
  behind.
- **A domain waiting for its deletion shows only its deletion state:** no
  "configuration file missing" badge, it is not measured for disk use, and
  it is not in the home page's certificate list.

### Updating from alpha.132

During this update the browser still runs alpha.132's own page, so the
update window can stay on its second step although the server has
finished. Reload the page to see the result. From alpha.133 on, the window
resumes by itself.

## Known limits

- **Site backups do not include a program's files and data.** A backup of
  a site holds its web folder (and its databases in a full backup), not
  the folder an uploaded program or Python app runs from or the data it
  keeps there. This is fixed in the next release. Until then: a scheduled
  **full** backup of the domain covers its databases; keep your own copy of
  the program's folder.
- **Deleting a domain removes the panel's records of its databases but
  leaves the databases themselves on the server.** The delete question says
  so; delete the databases first if they should go too. A choice in that
  question comes in the next release.
- **On a server installed before this version, the connection limits of
  database users may not be applied** (a plan's "connections per database
  user"). CelikPanel kept a copy of its own database account's password per
  customer, and older copies stopped working when the password changed.
  This release keeps every copy in step from now on, but it does not repair
  copies that are already out of step. To put them right, an administrator
  opens **Components → MariaDB** and, under "The account CelikPanel connects
  to this service with", chooses **More → New password…** once; then the
  same on **Components → PostgreSQL** if PostgreSQL is installed. Nothing
  else is affected: databases, sites and their users keep working, and
  deleting a database no longer depends on these copies. Automatic repair
  comes in the next release.
- If CelikPanel's own database account is removed on the MariaDB or
  PostgreSQL page, the next new customer opens it again. A fix comes in the
  next release.
- **Removing users on a database server reached with its own account**
  (another machine, or one an administrator registered) is not done in
  this release: it needs a check of the rights the owner gave there, which
  that account may not be allowed to read. The users stay and the screens
  say so.
- Some other database steps (suspending a subscription, database sizes,
  outside connections, the password change after a transfer) still decide
  whether a database server is on this machine by its address alone, and a
  database server registered once by its IP address and once by its name
  counts as two. Installing an application, copying a database and moving
  a database between PostgreSQL versions do not yet wait for a deletion on
  the same server. These come in a later release.
- While the panel and its Agent are from different versions (during an
  update), deleting a database removes the database but not yet its users:
  the database stays in the list with a note, and pressing **Delete** again
  after the update finishes it.
- **The size column is absent** from the Databases list until sizes can be
  read in one call for the whole list.
- **The certificate screen does not yet say whether the mail name is
  covered.**
- A deletion that stopped this way on alpha.132 finishes by itself within a
  few minutes after the update, whether the earlier deletion's check record
  is intact or was left half retired, as long as the record of that earlier
  deletion is still kept (the last 128 operations are kept). If it has
  dropped out, the deletion now says that the server administrator acts
  (send the two command outputs to support); there is no button for it
  yet.
- **A check record CelikPanel cannot prove ("unresolved") has no action in
  the panel yet.** The screen names the server administrator and the two
  read-only commands whose output goes to support; nothing is deleted or
  re-checked automatically. An action for the owner, with a typed
  confirmation, is planned. An earlier deletion whose job has dropped out
  of the last 128 kept operations lands in this state.
- An update worker that fails before the updater itself runs keeps its
  failed mark until a later update succeeds.
- The domain page's Advanced section can still offer "Recreate" for a
  domain that is waiting for its deletion.
- A deletion on a DNS pair also waits with no automatic way forward if,
  between two attempts, another zone is added or removed outside CelikPanel
  on the server (possible before this release too). A process stopped in
  the middle of a check record's write or retirement is no longer such a
  case; a power loss right after a write, before its data reached the disk,
  can still leave a file the next check cannot attribute, which then names
  the server administrator.
- A deletion that is stuck while the backup DNS server is still catching
  up has no retry on the page; the screen names support as the next step.
- Other waiting reasons of a deletion (for example a missing inspection
  set-up between the two servers) still show their earlier, longer text,
  some of which names "Retry this deletion"; that control is More →
  Delete… on the domain's row.
- A re-check that is held back by a DNS change made outside CelikPanel can
  still outlast the agent's lease; its result is then not counted and the
  next re-check decides.
- If a DNS change outside CelikPanel lands in the last one to three seconds
  of the proof, it is found afterwards: the deletion stays waiting and is
  retried, and it is never reported as done.
- A team member's Databases list is empty; their databases are on the
  pages of the domains they were given, as before.
- TO CONFIRM after the lab run: real-system check (update from alpha.132
  through the panel's own update screen; what was tried by hand, as which
  roles; what was not checked).
- Earlier limits still apply (see alpha.132).

## Evidence

No migration (schema stays 77). New endpoint `GET /api/v1/databases/visible`
(names, owners, domains, engines and user names only; never a password, a
connection secret or content). Deleting a database and setting a database
user's password now authorize by the visibility of the database's owner
instead of the caller's first subscription; a missing and a foreign record
answer alike; both audit rows name who acted and whose database it was
(`database.delete`). The stuck-deletion fix changes no stored format (the
journal sidecar and ledger stay version 1; the new reason codes
`dns_peer_proof_timeout:local_check` / `:peer_exchange` and
`dns_peer_journal_unresolved` use the existing error-code field, which older
readers treat as unknown). The check record's file steps changed, not its
content: a retirement is one rename to a private name, and the Agent
finishes or undoes what an earlier release left half done (RESILIENCE-CONTRACT,
"Branch a133/dnsproof4"). The deletion status gains the optional fields
`follow_up` and `next_check_at`. The Agent's blocklist answer gains the additive gob
fields Outcome, Reason and Sublists; an older Panel reads Listed, which a
current Agent sets only for a documented listing, and an older Agent's
answer is shown as "could not check". Release sequence 132 → 133.
