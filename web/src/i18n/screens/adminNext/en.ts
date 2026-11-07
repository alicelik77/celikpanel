// SCREEN catalogue part "adminNext" (alpha.127): what a screen says to the
// server's administrator instead of "the administrator does it" or "ask your
// provider", with the one next step it offers (components/AdminInstallLink.tsx),
// and the copy box of a command the administrator runs on the server
// (components/CopyCommand.tsx). Everyone else keeps their screen's own
// sentence, which says who acts. Key prefix kept here: adminNext.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "adminNext" ekran parçası: ekranın sunucu yöneticisine söylediği ve sunduğu
// tek sonraki adım; sunucuda çalıştırılacak komutun kopyalama kutusu.

export const enAdminNextScreens = {
    'adminNext.you': 'you (the server’s administrator)',
    'adminNext.openCatalog': 'Open Catalog management',
    'adminNext.openUsers': 'Open Users',
    'adminNext.openPHPFPM': 'Open PHP-FPM',
    'adminNext.openTool': 'Open {tool}',
    'adminNext.openMailSetup': 'Open the mail setup',
    'adminNext.openRights': 'Open {name}’s rights',
    'adminNext.openAccount': 'Open {name}’s account',
    'adminNext.access.notInPlan': 'File access is not part of this subscription’s plan. Give it in {name}’s rights; the site’s files are still available in the File manager.',
    'adminNext.access.suspended': 'This subscription is suspended, so access cannot be opened or changed. It can still be closed. Resume it from {name}’s account.',
    'adminNext.access.sshNotInPlan': 'The command line is not part of this subscription’s plan. File access is not affected; give it in {name}’s rights.',
    'adminNext.pool80': 'The customers of {reseller} used {used} of the {limit} monthly traffic pool. At 100% all of their websites show “traffic limit reached” until the 1st. Raise the pool in {reseller}’s rights.',
    'adminNext.pool100': 'The customers of {reseller} used the whole {limit} pool ({used}). Their websites show “traffic limit reached” until the 1st or until you raise the pool in {reseller}’s rights; mail and databases keep working.',
    'adminNext.runAsRoot': 'Run on the server as root:',
    'adminNext.onDebian': 'On Debian or Ubuntu, as root:',
    'adminNext.onArch': 'On Arch Linux, as root:',
    'adminNext.copied': 'Command copied.',
    'adminNext.copyFailed': 'The command could not be copied here. Select it and copy it by hand.',

    'adminNext.runtime.held': 'Uploaded programs are on a security hold on this server, so nothing can be created or started here. Lift the hold in Catalog management once it is safe; then come back here.',
    'adminNext.runtime.sandboxReduced': 'This server enforces a reduced sandbox (missing: {gaps}) that is not accepted for uploaded programs. Fix the server, or accept each missing protection in Catalog management; then a program can be created here.',
    'adminNext.runtime.notOffered': 'Uploaded programs are not offered on this server. Offer them in Catalog management if sites should use them; then a program can be created here.',
    'adminNext.runtime.notGranted': 'Uploaded programs are not part of this subscription. Include them in its plan under Users; then a program can be created here.',
    'adminNext.runtime.next.held': 'Lift the hold in Catalog management once it is safe. Files and settings are kept.',
    'adminNext.runtime.next.sandbox_reduced': 'Fix the server, or accept each missing protection in Catalog management.',
    'adminNext.runtime.next.not_offered': 'Offer uploaded programs in Catalog management and include them in this subscription’s plan under Users. Stop, log, environment and removal keep working.',
    'adminNext.runtime.next.suspended': 'Resume the subscription under Users.',
    'adminNext.runtime.next.oom': 'Make the program use less memory, or raise the memory limit in Catalog management. The log may show what grew.',
    'adminNext.runtime.next.wheel_unavailable': 'Choose a version of that package that publishes wheels for Python {minor}, or allow source builds for Python apps in Catalog management after reviewing their risk.',

    'adminNext.svcapps.held': 'This app is on a security hold on this server, so it cannot be installed or started. Lift the hold in Catalog management once it is safe.',
    'adminNext.svcapps.notOffered': 'Not offered on this server. Offer it in Catalog management if sites should use it.',

    'adminNext.mail.keySigningFailed': 'The DKIM key is ready, but DKIM signing could not be switched on, so mail still leaves unsigned. Press "Generate DKIM key" again; the key is kept. If it fails again, open the mail setup on Components and choose Repair.',
    'adminNext.mail.autoSigningFailed': 'The DKIM key is ready, but OpenDKIM could not be set up to sign with it (reason below), so mail still leaves unsigned. Press Try again; the key is kept. If it fails again, open the mail setup on Components and choose Repair.',

    'adminNext.access.noSSHD': 'This server has no OpenSSH server, which SFTP needs. Install it as root with the system’s package manager (openssh-server; openssh on Arch), then open this tab again.',
    'adminNext.access.jailProgramsNamed': 'Not installed on this server, so not in the command line: {names}. The rest of the command line works. Install them as root with the system’s package manager, then save access here again.',
    'adminNext.access.toolsUnknown': 'No package is known for {names} on this system: install it as root with the system’s package manager, then open this tab again.',
    'adminNext.access.confirmTitle': 'Install {packages}?',
    'adminNext.access.confirmMessage': 'CelikPanel installs the distribution’s official packages {packages} on this server, so the command line has its tools. Nothing else changes; FTP and SFTP keep working.',
    'adminNext.access.progressTitle': 'Installing {packages}',
    'adminNext.access.stepInstall': 'Installing the official packages: {packages}',
    'adminNext.access.stepCheck': 'Reading this site’s access again',
    'adminNext.webmail.not_installed': 'Webmail is off: Roundcube is not installed on this server. Install the Webmail setup; then give Roundcube its address, and every mailbox can open it.',
    'adminNext.webmail.no_address': 'Webmail is installed but has no address of its own yet, so mailboxes cannot open it. Give it one on the Roundcube page, for example mail.example.com; this card then shows it.',
    'adminNext.webmail.not_answering': 'Webmail has its address, but Roundcube does not answer there right now. Open the Roundcube page: it says why and how to start it again.',
    'adminNext.webmail.install': 'Install webmail',
    'adminNext.webmail.openRoundcube': 'Open Roundcube',
} as const;

export type AdminNextScreenKey = keyof typeof enAdminNextScreens;
