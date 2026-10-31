// SCREEN catalogue part "setupOwner" (alpha.114, owner review 2026-10-31):
// the main domain's website question and outcome, "Ready, with N things left
// for you", the banner of a wizard opened for a purpose, the fixed DNS
// software line of an extension, and the mail dialog on Components that adds
// mail in place (mail-only setup extension). Key prefixes: setup.v7.,
// setup.kind.main_site, svc.mailSetup.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "setupOwner": ana alan adının web sitesi, kalan
// işler, amaç bandı, sabit DNS yazılımı satırı ve yerinde posta penceresi.

export const enSetupOwnerScreens = {
    // ---- the main domain's website (first server only) ----
    'setup.v7.site.title': 'Will {domain}’s website be on this server?',
    'setup.v7.site.here': 'Yes, host it here',
    'setup.v7.site.here.tag': 'Recommended',
    'setup.v7.site.here.help': 'Setup ends with {domain} and www.{domain} opening from this server, secured with a free Let’s Encrypt certificate.',
    'setup.v7.site.elsewhere': 'No, its website is elsewhere',
    'setup.v7.site.elsewhere.help': '{domain} will only keep its DNS records here; its website is elsewhere. You can add the website later with “Host a website”.',
    'setup.v7.review.siteHere': '{domain} and www.{domain}: setup creates the website on this server, with a free certificate, as its last step. It opens once the domain points to your nameservers.',
    'setup.v7.review.siteElsewhere': '{domain} and www.{domain} will not open from this server: only their DNS records are kept here, and you imported no address for them. If the website runs elsewhere, import its address in Existing records, or the site stops opening once the nameserver change spreads.',
    'setup.kind.main_site': 'Create the website {target} and its certificate',
    // The mail dialog on Components lists every step kind a plan can hold; these two were only in the setup page's own copy.
    'setup.kind.infrastructure_dns': 'Prepare server address DNS records: {target}',
    'setup.kind.access_dns': 'Verify public DNS before certificate issuance: {target}',

    // ---- ready, with what is left ----
    'setup.v7.ready.tasksOne': '{server} is ready, with 1 thing left for you',
    'setup.v7.ready.tasksMany': '{server} is ready, with {count} things left for you',
    'setup.v7.ready.tasksHelp': 'Setup is finished and the services run. These are yours to do; each says what to do and where.',
    'setup.v7.site.done': '{domain} and www.{domain} are a website on this server, secured with a certificate.',
    'setup.v7.site.certPending': 'The website {domain} is created, but its certificate could not be issued yet: {reason} Once {domain} opens from this server, request the certificate on the domain’s SSL/TLS page.',
    'setup.v7.site.certWaitingOwner': 'The certificate of {domain} is issued, but the site’s configuration file is yours and does not use it yet. The SSL/TLS page says what to change.',
    'setup.v7.site.failed': 'The website {domain} could not be created: {reason} Add it with “Host a website” once that is fixed.',
    'setup.v7.site.dnsOnly': '{domain} already exists here without a website, so setup left it as it was. Open the domain to add hosting.',
    'setup.v7.site.reasonUnknown': 'no reason was given.',
    'setup.v7.task.registrar': 'Point {domain} to your nameservers at the company where you registered it: {servers}. Until then, {domain} and its names do not open from these servers.',
    'setup.v7.action.ssl': 'Open SSL/TLS',
    'setup.v7.action.domain': 'Open the domain',
    'setup.v7.action.dns': 'Open the DNS page',

    // ---- the wizard opened for a purpose ----
    'setup.v7.purpose.mail': 'You are here to add mail to this server; only the mail steps change.',
    'setup.v7.purpose.dkim': 'You are here to make your mail’s signing key (DKIM); only the mail steps change.',
    'setup.v7.purpose.back': 'Back to where you came from',

    // ---- the DNS software of a server that already runs one ----
    'setup.v7.engine.fixed': 'DNS software: {engine}, already running; it is not changed here.',

    // ---- Components: adding mail in place (mail-only setup extension) ----
    'svc.mailSetup.title': 'Install {name}',
    'svc.mailSetup.titleDKIM': 'Make the mail signing key (DKIM)',
    'svc.mailSetup.intro': 'Mail is added to this server’s finished setup, here in this window. Nothing else in the setup is asked or changed.',
    'svc.mailSetup.hostname': 'Mail host name',
    'svc.mailSetup.hostnameHelp': 'The name mail programs connect to, such as {proposal}. Setup points it to this server.',
    'svc.mailSetup.hostnameJoined': '{first} is the first server’s mail name and stays with {domain}. This server’s mail uses its own name, such as {proposal}.',
    'svc.mailSetup.hostnameTaken': '{name} is already used by the first server or your nameservers. Use another name, such as {proposal}.',
    'svc.mailSetup.hostnameInvalid': 'Enter a full name, such as {proposal}.',
    'svc.mailSetup.hostnameFixed': 'Mail host name: {name}',
    'svc.mailSetup.review': 'Show the steps',
    'svc.mailSetup.reading': 'Reading what setup will do. Nothing is changed yet.',
    'svc.mailSetup.stepsTitle': 'What happens, in order',
    'svc.mailSetup.blocked': 'These steps cannot start yet:',
    'svc.mailSetup.confirm': 'I have read the steps. Start them now.',
    'svc.mailSetup.start': 'Start',
    'svc.mailSetup.running': 'Mail is being installed. You can close this window; the work goes on and this page shows it again.',
    'svc.mailSetup.done': 'Mail is installed and secured with {host}.',
    'svc.mailSetup.doneDKIM': 'The mail steps finished. The DNS page shows the signing key and its record.',
    'svc.mailSetup.failed': 'Installing mail stopped.',
    'svc.mailSetup.failedNext': 'Nothing more is started. Open server setup to see the cause and change the plan, or stop and keep the server as it was before this window.',
    'svc.mailSetup.stop': 'Stop and keep the server as it was',
    'svc.mailSetup.openSetup': 'Open server setup',
    'svc.mailSetup.readFailed': 'The steps could not be read, so nothing was changed. Try again.',
    'svc.mailSetup.busy': 'Another setup run is in progress on this server. Wait until it finishes, then try again.',
    'svc.mailSetup.unknown': 'The server’s answer did not arrive. This window reads the state again; nothing is started twice.',
    'svc.mailSetup.close': 'Close',
} as const;

export type SetupOwnerScreenKey = keyof typeof enSetupOwnerScreens;
