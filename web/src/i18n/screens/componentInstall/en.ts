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
// services.mailProfiles.fallbackWarning.
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
    'services.notable.repoUnknown': 'vendor',
    'services.notable.repoName.sury': 'Sury PHP',
    'services.notable.repoName.pgdg': 'PostgreSQL (PGDG)',
    'services.notable.repoName.netdata': 'Netdata',
    'services.phpToolProblem.extensionsFailed': 'The PHP extensions {tool} needs could not be installed for PHP {version}, so {tool} may not work. Open {tool} in Components and choose PHP {version} again in Manage, or another version.',
    'services.mailProfiles.fallbackWarning': '{name} is installed. No domain has an active trusted Secure Mail certificate yet, so mail clients are served the server’s default certificate. They show a trust warning unless a trusted certificate covers the mail host name. To give a domain its own, activate a trusted certificate with Secure Mail enabled.',
} as const;

export type ComponentInstallScreenKey = keyof typeof enComponentInstallScreens;
