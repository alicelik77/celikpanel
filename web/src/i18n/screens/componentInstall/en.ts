// SCREEN catalogue part "componentInstall" (alpha.124): installing a
// component at component level - the install window's requirements, their
// versions and the repository change the owner confirms, the server-side
// install-together operation's words, and the notes a finished install says
// beyond "installed" (a PHP tool's proof or problem, a refused install's
// packages, ClamAV's first signatures, a mail profile on the default
// certificate). The notes moved here from the shell catalogue when the global
// operation provider started fetching them only when an operation ends
// (critical boot path). Key prefixes kept here: cinst., services.notable.,
// services.phpToolProof., services.phpToolProblem., and the keys
// services.installRefused.notable, services.installedWaitingSignatures and
// services.mailProfiles.fallbackWarning, and the audit words of the
// install-together operation (dashboard.audit.action.service.install.together*).
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "componentInstall" ekran parçası: bileşen düzeyinde kurulum ve biten
// kurulumun notları.

export const enComponentInstallScreens = {
    'services.phpToolProof.whyCheck': 'PHP-FPM {version} could not open CelikPanel’s check file (HTTP {status}); Check again in its Manage window repairs it',
    'services.phpToolProof.whySaid': 'it did not answer as itself; the server said: {detail}',
    'services.installedWaitingSignatures': '{name} is installed and is downloading its first virus signatures, which usually takes a few minutes. You do not need to do anything: its scanner starts by itself when the download finishes, and Components shows it as running at its next check.',
    'services.phpToolProblem.versionGone': '{tool} is set to PHP {version}, which is no longer installed, so {tool} cannot answer. Open {tool} in Components and choose an installed PHP version in Manage.',
    'services.phpToolProof.moved': '{tool} runs on PHP {version}: it did not work on PHP {first} ({why}), so the next installed PHP version that has its extensions was used. You can change it in {tool}’s Manage window.',
    'services.phpToolProof.notReady': '{tool} was installed, but it did not answer on PHP {version} ({why}). It reads Not ready: open its Manage window in Components; the reason is shown there, and Check again asks it again without moving it.',
    'services.phpToolProof.whyMissing': 'PHP {version} lacks {extensions}',
    'services.phpToolProof.whyOther': 'it did not answer as itself',
    'services.installRefused.notable': 'Nothing was installed: it would also have brought the packages below, which were not confirmed — the package manager’s answer changed after the install dialog was shown. Press Install again: the dialog now lists them for you to confirm.',
    'services.notable.why.new_php_version': 'part of another PHP version ({version})',
    'services.notable.why.apache': 'the Apache web server',
    'services.notable.why.component': 'another component: {component}',
    'services.notable.why.database_server': 'a database server',
    'services.notable.why.upgrade': 'already installed; would be upgraded although the install did not ask for it',
    'services.notable.why.vendor_shared': 'the {repo} repository’s version instead of the distribution’s',
    'services.notable.repoUnknown': 'non-distribution',
    'services.notable.repoName.sury': 'Sury PHP',
    'services.notable.repoName.pgdg': 'PostgreSQL (PGDG)',
    'services.notable.repoName.netdata': 'Netdata',
    'services.phpToolProblem.extensionsFailed': 'The PHP extensions {tool} needs could not be installed for PHP {version}, so {tool} may not work. Open {tool} in Components and choose PHP {version} again in Manage, or another version.',
    'services.mailProfiles.fallbackWarning': '{name} is installed. No domain has an active trusted Secure Mail certificate yet, so mail clients are served the server’s default certificate. They show a trust warning unless a trusted certificate covers the mail host name. To give a domain its own, activate a trusted certificate with Secure Mail enabled.',
    'cinst.requiresTitle': 'Requires',
    'cinst.checking': 'Checking what {name} needs…',
    'cinst.unknownPlan': 'What {name} needs could not be checked now. Nothing was changed. Press Retry; Install opens when the answer is in.',
    'cinst.nothing': 'Nothing else: it works on its own.',
    'cinst.installed': 'Installed',
    'cinst.installedVersion': 'Installed: {version}',
    'cinst.together': 'Not installed yet: installed first, together with {name}, at the distribution’s version.',
    'cinst.unknownNeed': 'Not known yet whether it is installed. Scan this server on this page, then open this window again.',
    'cinst.forOther': '(for {name})',
    'cinst.reason.mail_setup': 'Not installed. It comes with the mail setup: set up mail with the Email card on this page first, then install {name}.',
    'cinst.reason.not_offered': 'Not installed, and {component} is not offered on this system, so {name} cannot be installed here.',
    'cinst.reason.conflict': 'Not installed: {holder} holds the same role on this server. Remove {holder} first if {name} should use {component}.',
    'cinst.reason.elsewhere': 'Not installed. Install {component} from its own row on this page first, then install {name}.',
    'cinst.blocked': 'Install is off: {name} needs something that cannot be installed from this window. Who acts: you. Next: do what the line marked below says, then open this window again.',
    'cinst.togetherSummary': 'Install installs {names} first, then {name}, as one operation on the server. It goes on if this page closes; if one step fails, what was installed before it stays installed and the window says so.',
    'cinst.phpTitle': 'PHP version {name} runs on',
    'cinst.phpDefault': 'PHP {version} (the distribution’s)',
    'cinst.phpHint': 'Only installed versions are listed. You can move {name} to another version later in its Manage window.',
    'cinst.brings.component': '{component} (another component)',
    'cinst.brings.new_php_version': 'PHP {version}, beside the PHP already here',
    'cinst.brings.apache': 'The Apache web server',
    'cinst.brings.database_server': 'A database server',
    'cinst.brings.upgrade': '{count} system package(s) already installed, updated to a newer version',
    'cinst.bindFailed': '{tool} is installed, but it could not be moved to PHP {version}{why}, so it runs on PHP {previous}. Who acts: you. Next: choose PHP {version} again in {tool}’s Manage window, or keep PHP {previous}.',
    'dashboard.audit.action.service.install.together': 'installed {target} together with what it needed',
    'dashboard.audit.action.service.install.together.failed': 'could not finish installing {target} together with what it needed',
    'dashboard.audit.action.service.install.together.recovered': 'finished installing {target} together after a restart',
    'dashboard.audit.action.service.install.together.recovered.failed': 'could not finish installing {target} together after a restart',
} as const;

export type ComponentInstallScreenKey = keyof typeof enComponentInstallScreens;
