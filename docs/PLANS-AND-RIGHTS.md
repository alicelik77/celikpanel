# Plans, rights and the room to play

*Design document · [Türkçe](PLANS-AND-RIGHTS.tr.md)*

*November 2, 2026 · Status: **APPROVED** (owner, November 2, 2026: "Onay veriyorum" - "I approve"). Not yet
implemented; implementation follows the order in Section 13. This document does not describe
anything that has been built.*

This document gathers the direction and decisions the owner gave on November 2, 2026 in one
place. It extends, but does not change, two decisions taken in July: D-014 ("the administrator
installs, the reseller offers, the customer uses; the unit is the version") and D-017 (every
choice flows through one offering-ID space). It follows the version principle (D-038: what is in
use is not removed, nobody is moved silently), owner independence (D-022) and the actionable
guidance rule (D-024).

---

## 1. The owner's words (November 2, 2026)

- "The administrator must decide what a reseller or a customer can be offered or can use. The
  reseller must also be able to choose what their customer can use." (Turkish original: "Reseller
  ya da müşteriye neleri sunabileceğini veya kullanabileceğini admin belirlemeli. Reseller da
  müşterisinin neleri kullanabileceğini seçebilmeli.")
- "The administrator installing the Node.js component on the server does not mean a reseller or a
  customer can use it. Resellers and customers can play in the playground the administrator
  prepared."
- "A customer defines Node.js for one of their domains, defines PHP 8.5. Or does not give the
  Node.js right and gives PHP 8.3."
- "Having everything unlimited is also a plan. Perhaps it is the basic plan."
- "A plan is really a template. Not everyone has to be on a plan. You can also choose and set
  things one by one without choosing a plan."
- "If even one thing is made different from the plan it is bound to, it leaves the plan and
  becomes its own, custom. It is not 'plan + something'."
- "A reseller can define their own plans within the rights I give them."
- "Design this very carefully and precisely. Do not start work without approving everything."

---

## 2. What exists today, and what does not (from the code, November 2, 2026)

**Exists:**
- Plans (`service_plans`): numbers only. Domain, database, mailbox and disk limits. There is also
  a traffic field but it is not measured. 0 means unlimited.
- Subscription (`subscriptions`): copies the plan's numbers.
- Reseller pool (`reseller_pools`): totals the reseller can hand out.
- A reseller can make their own plans, but only as numbers.
- The administrator has one "versions shown to customers" list for the whole server; only for PHP
  and Node.js.
- Two product permissions: VPN and the app installer.
- A detailed permission model for additional users; this draft does not touch it.

**Does not exist:**
- Which jobs (DNS editing, scheduled tasks, backups...) are open in a plan.
- Which software and version is open in a plan.
- Transfer of a subscription.
- Hiding a feature from the menu when it is not installed or not permitted.

**Defects found during this work** (to be fixed in a separate small release, not tied to this
draft):
1. A reseller is created unlimited without being asked.
2. Creating a database from the domain page has no quota check.
3. Editing a plan has no check for going below current usage.
4. The target address of a proxy site is unrestricted. This is a security hole: services of the
   server that are open only internally could be exposed outward through a customer's domain.
5. The VPN page is shown wrongly to the reseller and the customer.

---

## 3. Principles

**P1 - Installed does not grant.** When the administrator installs a component, it only becomes
*grantable*. For an account to use something, it must be open in the account's **rights set**.

**P2 - The administrator draws the field, the reseller only narrows.** Everything a customer can
use is the intersection of four layers:

```
installed  ∩  offered by the administrator  ∩  the reseller's rights set  ∩  the subscription's rights set
```

A reseller cannot give any customer a right the reseller does not have. Not by a plan, not by a
custom setting, not by a transfer.

**P3 - Every account's rights either come from a plan or are chosen for that account.** (Owner,
November 2, 2026; replaces the ROLES.md rule "no hand-made permission sets for resellers and
customers".) There are three states, and every screen names them:
- **Unlimited:** If no plan is chosen and nothing is set specially, the account is on the
  Unlimited plan. Owner: "Those not bound to a plan are included in the Unlimited plan."
- **Bound to a plan:** The rights set is exactly the same as the plan's. When the plan changes,
  the account changes.
- **Custom:** The rights set belongs to the account. Either it was chosen item by item without a
  plan, or even a single thing was changed while it was bound to a plan. At that moment the
  account leaves the plan and becomes "Custom". There is no in-between state "plan + extra".

**The purpose of a plan is speed.** Owner: "Choosing a plan is to paste the rights onto the
reseller or customer quickly as a template. Otherwise everything would have to be chosen one by
one for every reseller or customer." A plan is not a requirement; it is a ready-made template.

**P4 - "Unlimited" is also a plan, and its name describes its behavior.** The panel comes with
two ready-made plans: **Unlimited** for customers and **Unlimited reseller** for resellers. An
Unlimited plan opens everything the administrator offers and sets no numeric limit. When the
administrator offers something new (a new PHP version, a new right, a new piece of software), the
Unlimited plan takes it **automatically**. Owner, November 2, 2026: "If a new thing or feature
comes and is not added to this plan, its name is not Unlimited; it is plan A." Therefore the items
of an Unlimited plan cannot be chosen one by one. If someone wants to narrow it, the plan stops
being Unlimited: the administrator derives a new, differently named plan from it. The Unlimited
plan appears in the form by name; it is not a hidden default.

**P5 - Narrowing does not break what runs** (D-038). When an item is closed, a site or service
that uses it keeps running. Only new use is refused. The screen says who must do what. No site is
silently moved to another version.

**P6 - The rule is enforced on the server; the screen only shows.** Everything hidden on screen
is also refused on the server. The cPanel defect "the PHP version is restricted on screen but can
be bypassed with a file" is not repeated.

**P7 - No silent change.** Changing a plan, turning a subscription custom, binding it to a plan
and transferring it are each a change. Each first shows what will change, then asks for approval,
and is written to the audit log.

---

## 4. Contents of the rights set

The rights set has three parts. A plan and a custom rights set are made of the same three parts.

### 4.1 Quantities

| Item | Note |
|---|---|
| Number of domains | Subdomains count too (as today) |
| Number of databases | Checked on every creation path (today one path is missing) |
| Number of mailboxes | |
| Disk | Today a soft gate at creation time; OS quota is a separate job |
| Traffic | **Not shown** in a plan until it is measured (honesty rule) |
| Number of customers | Only in a reseller's rights set |

An item left at 0 or empty means **unlimited**, and the screen says "unlimited".

### 4.2 Permissions (which jobs)

| Permission | If closed |
|---|---|
| Editing DNS records | Records are visible, cannot be changed |
| DNSSEC | The section is not visible |
| Mail (mailboxes, forwarding, auto-reply) | The Mail tab is not visible |
| Database | The Databases menu and tab are not visible |
| Scheduled tasks | The tab is not visible |
| File manager | The tab is not visible |
| Backup | The tab is not visible |
| Certificate: free (ACME) | A certificate cannot be obtained |
| Certificate: uploading your own | The upload tab is not visible |
| Protected folders, redirects | The tabs are not visible |
| Site type: proxy | The option is not in the list |
| App installer (WordPress) | Replaces today's product permission |
| VPN | The menu is not visible; replaces today's product permission |
| Additional users (team) | The menu is not visible |
| File access (FTP and SFTP) | The Access tab is not visible; files are uploaded only through the panel's file manager |
| SSH (command line) | The command line cannot be entered; file access is not affected |

**File access does not exist today.** On November 2, 2026 the owner looked for the FTP and SSH
settings of a domain and could not find them. Verified from the code: there is no path in the
panel that creates an FTP, SFTP or SSH account; installation of the FTP component is closed
("per-subscription access is not wired yet"). This will be built as a separate feature.

Owner: "FTP and SSH are the most basic connection methods; how will files be uploaded? SSH may be
closed for security, access may not be allowed; this is subject to permission. Security is
important. They should be able to play only in their own sandbox; they must not connect and haunt
the server." (Turkish original: "FTP ve SSH en temel bağlantı şekli; dosya nasıl yüklenecek? SSH
güvenlik gereği kapalı olabilir, erişime izin verilmeyebilir; bu izne tabi. Güvenlik önemli.
Sadece kendi kumunda oynayabilmeli; bağlanıp sunucuya musallat olmamalı.")

Proposed form:
- Every subscription gets an **Access** tab: user name, password or SSH key, connection address
  and port, a short connection recipe for programs like FileZilla.
- **File access (FTP and SFTP)** is one permission; it is open in the Unlimited plan.
  - **FTP:** The connection is encrypted (FTP over TLS). Programs like FileZilla use this
    automatically; for the customer there is no difference. The old plain FTP, where the password
    travels unencrypted, is not used.
  - **SFTP:** File transfer over an SSH connection with the same user name and password. It does
    not open a command line.
- **SSH (command line)** is a separate permission and is subject to permission. The administrator
  can also turn it off for the whole server.
- **It stays in its own sandbox.** A user who connects with FTP, SFTP or SSH sees only their own
  subscription's folder. They cannot see the rest of the server, other customers' files or system
  files; they cannot run administrator commands. Even when SSH is open, the user is in an
  environment closed to their own folder (chroot), and only the permitted basic tools are in it.
  Everything that could harm the server (other users' processes, system settings, network
  settings) is outside this environment.
- **Against brute force:** Failed logins are blocked with fail2ban; password rules are the same as
  the panel's account passwords; an SSH key is recommended.
- On a suspended subscription all access is closed (Section 9A).
- Today site users are created on the server with the `/bin/bash` shell
  (`internal/services/user_manager.go`). They cannot log in today because they have no password,
  but before the Access feature this default will be made safe: a user without the SSH permission
  has no command-line shell.

### 4.3 Software (which component, which version)

The offering IDs of D-017 are used. The unit is the version (D-014).

| Item | Example ID |
|---|---|
| PHP versions | `component:php-fpm:8.3`, `component:php-fpm:8.5` |
| Node.js versions | `component:node:22` |
| Database engines | `component:mariadb`, `component:postgresql:16` |
| Webmail | `component:roundcube` |
| Certificate providers | `integration:acme:letsencrypt` |

An item can be added to a rights set only if it is installed on the server and offered by the
administrator. If a software item is not chosen, that software is closed. For example, if no
Node.js version is chosen, the "Node.js" site type does not appear in the list. The Unlimited plan
automatically covers every item the administrator offers; when a new PHP version is installed and
offered on the server, those on the Unlimited plan see it at once.

**Two example plans:**

| | Basic | Developer |
|---|---|---|
| PHP | 8.3 | 8.3, 8.5 |
| Node.js | none | 20, 22 |
| Database | MariaDB | MariaDB, PostgreSQL 16 |
| DNS editing | none | yes |
| Scheduled tasks | yes | yes |

### 4.4 Third-party software (later)

Owner, November 2, 2026: "Later we will allow third-party software too; these will also be in or
out of these rights. Security is important, let us be careful." (Turkish original: "Daha sonra
üçüncü parti yazılımlara da izin vereceğiz; bunlar da bu haklarda olacak veya olmayacak. Güvenlik
önemli, dikkat edelim.")

Every piece of software that comes from a store or from a vendor is a separate item in the rights
set. The same intersection rule applies; no separate permission system is built for third-party
software (D-017: one offering-ID space). Example ID: `extension:<vendor>:<name>`. Extra rules for
security:

- **Installed is not offered.** When third-party software is installed, it is open to no account.
  The administrator first moves it into the **offered** layer; in doing so they see and approve
  the permissions the software asks for. At the moment it is offered, the Unlimited plan takes it
  automatically (P4); the administrator or the reseller adds it to other plans and custom rights
  sets knowingly. The security gate is thus at the "offering" step, and the name of the Unlimited
  plan stays true.
- **It states in advance what it will touch.** Third-party software lists the permissions it needs
  openly in a manifest; for example "reads the customer's files", "creates databases", "makes
  outside connections". The administrator sees and approves this list before installing. It
  cannot do anything that is not in the list.
- **It cannot go outside the rights set.** While third-party software works on behalf of a
  customer, it is limited by that customer's rights set. It cannot grant rights, widen rights or
  change plans on its own.
- **The gate is in the panel.** The permission check is made by the panel, not by the third-party
  software. Even if the software's own screen says "allowed", nothing happens if the panel
  refuses.
- **Signed and recorded.** Only packages whose signature is verified can be installed (the same
  as the current release-signature arrangement). The installation, the permission approval and
  every plan change are written to the audit log.
- **Removal does not break what runs, but security comes first.** Third-party software in use
  normally cannot be removed (D-038). But when a security problem is reported, the administrator
  can **stop** it at once; what will be affected is written on the screen.

This section sets a principle; the store and the third-party package arrangement are a separate
design.

---

## 5. Roles and the room to play

**Administrator**
- Chooses what will be installed on the server.
- Chooses which of the installed things are **offered**. Today's "versions shown to customers"
  list becomes this layer and also covers items other than PHP/Node.js.
- Makes customer and reseller plans. Binds a plan to every account or gives it a custom rights
  set.
- Their own working area is unlimited (as today).

**Reseller**
- Their own rights set comes from the reseller plan the administrator bound to them or from the
  custom rights set the administrator gave them. A reseller's rights set holds today's pool
  (totals and number of customers) and the permission and software items the reseller can hand
  out, together.
- Makes their own plans. A reseller plan cannot go outside the reseller's own rights set.
- Binds to their customers one of their own plans or one of the general plans the administrator
  opened to the reseller, or gives a custom rights set.
- **The pool limits real usage, not distribution.** Owner: "You give a reseller a 1 TB data
  transfer allowance, the reseller gives 500 GB each to 10 customers. In theory the total passes
  but in practice it must not be able to pass." (Turkish original: "Bayiye 1 TB veri transferi
  izni verirsin, bayi 10 müşterisine her biri için 500 GB verir. Toplam teoride geçer ama
  pratikte geçememeli.") That is, the reseller may distribute more than the pool to their
  customers; but the **real** total usage of all their customers cannot exceed the pool. When the
  total reaches the pool, none of that reseller's customers can use more; new use is refused, with
  who must do what written on the screen. At 80% and at 100% a warning goes to the reseller and
  the administrator. This is the opposite of today's rule: today the distributed total is
  limited.
- For the count items (domain, database, mailbox, customer) real usage is already a number: what
  has been created is counted.
- Until traffic starts to be measured, the traffic pool is not applied and the screen says so.

**Customer**
- Sees and uses only what is open in their own rights set.
- In the "My plan" card they see the name of their plan or the word "Custom", its limits and
  their usage. It also says whom to contact for more.

**Additional user** (today's model): Gets a subset of the customer's rights set. An additional
user can never get a permission the customer does not have.

---

## 6. Plan and subscription behavior

**6.1 Creating an account.** The "Rights" section of the form is always visible. There are three
ways:
- **Unlimited:** It comes selected, by name. If nothing is changed, the account becomes
  Unlimited. This is a choice that is seen on purpose; it is not silent as today.
- **Choose a plan:** The list shows the plans the creator can give.
- **Set custom:** The three parts are filled in one by one. Only items in the creator's own rights
  set can be chosen.

The same section appears when creating a reseller: Unlimited reseller, a reseller plan, or a
custom reseller rights set. For a customer created by a reseller, "Unlimited" means the whole of
that reseller's own rights set; it does not go outside the reseller's field.

**6.2 Going from plan to custom.** When a single item is changed on a subscription bound to a
plan, this is asked before saving:

> With this change the account leaves the "Basic" plan and becomes "Custom". Later changes to the
> plan will not reach this account. Continue?

**6.3 Going from custom back to a plan.** A custom subscription can be bound to a plan. The
differences are shown first; if there are items where usage is larger than the plan, today's
"usage is above the plan" rule applies. Nothing is deleted; only new use is blocked.

**6.4 Editing a plan.** Before saving, this is shown:
- How many subscriptions are bound to this plan.
- In which subscriptions the narrowed items are in use. For example: "PHP 8.5 is being removed;
  it is used in 3 sites in this plan. The sites will keep running; it cannot be chosen in a new
  site."
- The subscriptions whose quantity falls below usage.

Custom subscriptions do not appear on this screen, because they have left the plan.

**6.5 Deleting a plan.** It is refused if a subscription is bound to the plan (as today). The two
ready-made Unlimited plans cannot be deleted. The administrator can change their limits and
items.

**6.6 If a reseller's rights set narrows.** If the administrator removes an item from a reseller's
rights set, the reseller's plans and customers may keep using that item (P5). But none of them can
choose that item again. On the reseller's screen, the affected plans and customers are listed.

---

## 7. Removal from the server

Today removing a PHP or Node.js version is refused if sites use it. This rule widens: the removal
screen also lists the **plans and custom rights sets** that keep that item open. They do not
block the removal; it is only shown as "this item will drop from these plans too". The only thing
that blocks is still real use: the sites that use that version.

---

## 8. Transfer

**Unit.** What is transferred is a **subscription**: the domains, sites, mailboxes, databases,
certificates and scheduled tasks in it go together. Moving a single domain to another subscription
is a separate job and is outside this draft.

**Who can transfer.**
- Administrator: from anyone to anyone.
- Reseller: only among their own customers.
- Customer: cannot transfer on their own (see Decision K3).
- Transfer between customers of two different resellers is done only by the administrator.

**Pre-check** (before anything changes):
1. **The ceiling of the receiving side:** The subscription goes with its rights set. If the
   receiving side is bound to a reseller, that reseller's pool must have free room for the
   quantities this subscription gives. Every permission and software item the subscription keeps
   open must be in the reseller's rights set.
2. **If it does not fit, the transfer does not start.** The screen writes each missing item and
   the ways out:
   - The administrator widens the reseller's rights set.
   - Or the subscription is narrowed first. This is possible only if real usage fits the narrowed
     rights set.
   - Or the subscription stays where it is.
3. **Additional users:** The permissions of the old owner's additional users in this subscription
   are removed in the transfer. It is written on the screen in advance.
4. **Preview:** What passes to whom, which permissions will be removed and what will change in the
   pools is shown, then approval is asked.

**Server side.** Whether the transfer needs to change files, system users or service
configuration on the server will be verified in the implementation design (see Section 11). If it
does, the transfer becomes an operation that follows the change rules of D-025: preview, durable
checkpoints, rollback.

---

## 9. Migration of existing installations

No site, mail, database or service changes. Only the panel's records change.

| Today | After migration |
|---|---|
| Subscription bound to a plan | Bound to the same plan. The plan keeps today's numbers; the permission and software parts are filled as "everything the administrator offers". Behavior does not change. |
| Subscription without a plan (numbers by hand) | Custom. Today's numbers are kept; permissions and software are "everything offered". |
| Customer without a subscription | Unlimited (P3: one not bound to a plan is on Unlimited). Today this account cannot create anything; after migration it can. **This is a behavior change;** after migration the administrator is shown the list of these accounts. |
| Reseller without a pool | Unlimited reseller. Behavior does not change. |
| Reseller with a pool | Custom reseller rights set: today's pool numbers; permissions and software "everything offered". |
| VPN and app-installer product permissions | As permissions in the subscription's rights set. Closed for those without the permission. |
| "Versions shown to customers" list | The administrator's "offered" layer. |

After migration the administrator finds everything as it is today, but now sees which account is
on which plan or custom, and can start narrowing.

**Going back.** New tables are added; old columns are not deleted. If one goes back to the
previous release, the old panel does not see the new tables and works as it does today. In the
implementation phase this is proven by a rollback trial, as D-025 requires.

---

## 9A. When a subscription ends: suspension

Owner, November 2, 2026: "If a customer's or a reseller's subscription ends, all the related
sites must be suspended. If something is suspended, it says 'this site has been suspended'. No
service, whether VPN or database, serves. Maybe they are not using it as hosting but only as a
database; we must not allow that." (Turkish original: "Müşterinin ya da bayinin aboneliği biterse
ilgili tüm siteler askıya alınmalı. Bir şey askıya alındıysa 'bu site askıya alınmıştır' denir.
Hiçbir servis, VPN olsun veritabanı olsun, hizmet vermez. Belki hosting değil sadece veritabanı
olarak kullanıyor olabilir; izin vermemek lazım.") Earlier answer: "After a certain time it is
deleted by hand."

**When.** When the subscription's end date has passed, or when the administrator or the reseller
ends the subscription. When a reseller's subscription ends, the subscriptions of all of that
reseller's customers are also suspended.

**What happens.** No service of the suspended subscription works:

| Service | While suspended |
|---|---|
| Websites | The visitor sees the "This site has been suspended" page |
| Mail | Incoming mail is not accepted; mailboxes cannot be logged in to |
| Databases | Database users cannot connect |
| VPN | Devices cannot connect |
| Scheduled tasks | Do not run |
| File access (FTP/SFTP) | Login is not possible |
| Panel | When the account owner logs in, they see only the "Your subscription is suspended; contact this person" screen |

**Nothing is deleted.** Files, mailboxes, databases and configuration stay as they are. When the
subscription is restarted, everything comes back exactly.

**Deletion is by hand.** A suspended subscription appears at once in the administrator's or the
reseller's "deletable" list (K6). Only a person starts a deletion; the panel never deletes on its
own. Before deleting, what will be deleted (sites, mailboxes, databases, backups) is shown and
approval is asked.

**Safety and resilience.**
- Suspending and resuming are changes (D-025): what will change is shown first, it is done on the
  server with checkpoints, and if interrupted the same operation continues where it stopped.
- Suspension is done with each service's own standard configuration (D-022). Even if the panel is
  removed, a suspended site stays suspended and an open site stays open.
- In today's panel there is no suspending of a site; an account can be frozen, but the sites keep
  running. This section is a new part.

---

## 10. Lessons from Plesk and cPanel

Sources: Plesk Obsidian and cPanel/WHM documentation and support articles (research of November
2, 2026). Points that could not be verified stayed uncertain.

| Problem seen there | The counterpart in this design |
|---|---|
| Plesk: a subscription detached from its plan stays detached silently, plan changes do not reach it and this goes unnoticed | There are only two states and every screen names them: "Plan: Basic" or "Custom". Detaching happens with a confirmation question. |
| Plesk: the plan sync error comes late and without guidance ("limitations exceeded") | Editing a plan shows those affected before saving; the refusal message says who does what (D-024). |
| Plesk: sites using a removed PHP version give 503; old references stay in plans | A version in use cannot be removed; the removal screen shows plans too. |
| cPanel: the PHP version restriction is only on screen and is bypassed with `.htaccess` | The rule is enforced on the server (P6). |
| cPanel: an account can detach from its package, and it is unclear whether a package change reaches the accounts | An account bound to a plan is always the same as the plan; a custom account is never affected. There is no in-between state. |
| cPanel: reseller limits are off by default and invisible | Every reseller's state is written by name: Unlimited reseller, a plan or Custom. Unlimitedness is a visible choice. |
| Plesk: on transfer the receiving reseller's capacity is checked (a good example) | The same, with permission and software items added. |

---

## 11. To be verified in the implementation

This draft was prepared by reading the code. Before implementation starts, the following will be
verified:
- With which system user do a subscription's sites run on the server; does file ownership need to
  change on transfer?
- How will the additional-user triggers (`029_additional_users.sql`) allow the owner to change on
  transfer?
- Does every create and change path go through a single check? Today one database path skips the
  quota check; such paths will be listed one by one.
- How will the Unlimited plan "automatically covering newly installed versions" and the custom
  rights sets "covering only what is chosen" be stored under the same intersection rule?

---

## 12. Decisions the owner had to give

- **K1 - Role rule. DECIDED:** "Every account's rights either come from a plan or are chosen for
  that account." ROLES.md will be updated accordingly (P3).
- **K2 - Overselling. DECIDED:** The distribution may exceed the pool; the real total usage may
  not (Section 5, Reseller).
- **K3 - A customer's transfer request. DECIDED:** The customer sends a request; their reseller or
  the administrator approves.
- **K4 - Accounts without a plan. DECIDED:** An account not bound to a plan and without custom
  rights is on Unlimited (P3). When the subscription ends it is suspended, and after a certain
  time it is deleted by hand (Section 9A).
- **K5 - The Unlimited plan and new arrivals. DECIDED (owner, November 2, 2026):** The Unlimited
  plan automatically takes everything new that the administrator offers; if it does not, its name
  cannot be Unlimited. For third-party software the security gate is the "offering" step (Section
  4.4).
- **K6 - The deletable list. DECIDED:** Owner: "Let the suspended account go straight onto the
  deletable list; we will delete by hand anyway." (Turkish original: "Askıya alınan hesap hemen
  silinebilir listesine düşsün; zaten elle sileceğiz.") There is no waiting period; deletion is
  always by hand and with a preview.
- **K7 - End date. DECIDED:** Owner: "In the future there will be a payment and subscription
  system, prepare for it. The end date can be infinite for now, but when the subscription system
  comes there will be an end date." (Turkish original: "İleride ödeme ve abonelik sistemi olacak,
  ona göre hazırlık yap. Bitiş tarihi şimdilik sonsuz olabilir, ama abonelik sistemi gelince
  bitiş tarihi olacak.") Today every subscription has a start date and an end date that **may be
  empty**; if empty, the subscription is infinite and is ended only by hand. The administrator and
  the reseller can enter an end date. When the payment system comes, it will manage the end date;
  suspension (Section 9A) uses the same path. As preparation, the subscription record carries
  these fields from the start: start, end (may be empty), status (active, suspended, ended),
  suspension reason (expired, manual, payment) and suspension time. Price, period and payment
  fields come with the payment system; they are not added now as empty fields (honesty rule).

---

## 13. Implementation (owner, November 2, 2026: "hepsini yap" - "do all of it")

All the parts ship in **one release** (alpha.117); the parts are written at the same time and
tried once at the end of the release:
1. **Data model and migration:** rights set tables, the two Unlimited plans, migration of existing
   records, subscription dates and status.
2. **Server-side gates:** every choice and creation path applies the intersection rule; refusal
   messages say who does what; the reseller pool limits real usage.
3. **Screens:** the account creation form, the plan editing preview, the subscription page, menu
   and tab hiding, the "My plan" card.
4. **Suspension:** with each service's own configuration, reversible.
5. **Transfer:** pre-check, preview, request and approval.
6. **File access:** FTP (TLS), SFTP, permission-bound SSH; closed to its own folder.

Trial: tests of the changed parts, GitHub checks, and once at the end of the release an update, a
rollback, a suspension and an SSH sandbox trial on a real server.

**Out of scope:** price, period and payment; traffic measurement; OS disk quota; moving a single
domain between subscriptions.
