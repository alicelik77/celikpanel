// SCREEN catalogue part "domainSections" (alpha.132): the nine sections of a
// domain's page - the row of sections, the "On this page" index of a
// section's parts, the view-only note of a team member, and the page of a
// section the plan does not include (whom to ask, what to ask for).
// Key prefixes kept here: domain.section., domain.part., domain.notInPlan.,
// and domain.sections, domain.onThisPage, domain.readOnly. Every other
// domain. key stays in "sites".
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "domainSections": alan adı sayfasının dokuz bölümü.

export const enDomainSectionsScreens = {
    'domain.sections': 'Sections of this domain',
    'domain.section.overview': 'Overview',
    'domain.section.site': 'Site',
    'domain.section.files': 'Files',
    'domain.section.mail': 'Mail',
    'domain.section.databases': 'Databases',
    'domain.section.dns': 'Domain & DNS',
    'domain.section.security': 'Security',
    'domain.section.backups': 'Backups',
    'domain.section.advanced': 'Advanced',
    'domain.section.notInPlan': 'not in the plan',
    'domain.onThisPage': 'On this page:',
    'domain.part.type': 'Site type',
    'domain.part.apps': 'Applications',
    'domain.part.php': 'PHP settings',
    'domain.part.fileManager': 'File manager',
    'domain.part.access': 'FTP, SFTP and SSH',
    'domain.part.connection': 'Connection',
    'domain.part.names': 'Names',
    'domain.part.records': 'DNS records',
    'domain.part.certificate': 'Certificate (HTTPS)',
    'domain.part.cron': 'Scheduled tasks',
    'domain.part.logs': 'Logs',
    // alpha.132 (owner review, 12 Nov 2026): the parts of Site and Advanced are cards that open and close;
    // each card's one sentence (domain.about.*) is in the "sites" part.
    'domain.part.toggleOpen': 'Open',
    'domain.part.toggleClose': 'Close',
    'domain.readOnly': 'You can only view this. To change it, ask the account owner for permission.',
    'domain.notInPlan.title': '{section} is not in your plan',
    'domain.notInPlan.titleDomain': '{section} is not in this domain’s plan',
    'domain.notInPlan.titleAccount': '{section} is not in this account’s plan',
    'domain.notInPlan.about.mail': 'Mail gives this domain its own mailboxes and forwarding addresses.',
    'domain.notInPlan.about.databases': 'Databases hold the data of your site and its applications.',
    'domain.notInPlan.about.backups': 'Backups keep a copy of your site’s files and databases, so you can restore them when you need to.',
    'domain.notInPlan.about.files': 'Files lets you manage your site’s files here and reach them over FTP or SFTP.',
    'domain.notInPlan.whoLabel': 'Who can add it',
    'domain.notInPlan.askLabel': 'What to ask for',
    'domain.notInPlan.addLabel': 'What to add',
    'domain.notInPlan.thenLabel': 'Once it is added',
    'domain.notInPlan.who.provider': 'Your provider: {who}',
    'domain.notInPlan.who.administrator': 'The server administrator: {who}',
    'domain.notInPlan.who.providerUnnamed': 'Your provider',
    'domain.notInPlan.who.reseller': 'If this is a customer’s domain, you can, in the account’s rights on the Users page. If it is your own, the server administrator can.',
    'domain.notInPlan.who.resellerNamed': 'If this is a customer’s domain, you can, in the account’s rights on the Users page. If it is your own, the server administrator can: {who}.',
    'domain.notInPlan.who.team': 'The account owner, who can ask their provider for it.',
    'domain.notInPlan.ask.own': '“{right}” added to your plan.',
    'domain.notInPlan.ask.team': '“{right}” added to the account’s plan.',
    'domain.notInPlan.ask.reseller': 'The right “{right}”.',
    'domain.notInPlan.then': 'This section opens here. Nothing needs to be installed.',
    'domain.notInPlan.openUsers': 'Open Users',
} as const;

export type DomainSectionsScreenKey = keyof typeof enDomainSectionsScreens;
