// SCREEN catalogue part "vendorLanes": the operating system's packages by
// default, a vendor repository only as the administrator's informed choice
// (v0.1.0-alpha.108, D-038 amendment "two lanes"): the lane dialog, the
// repository card's missing steps, locked and no-longer-updated versions,
// tested marks and the security updates card's direct action. Key prefix
// kept here: lane.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "vendorLanes": varsayılan olarak işletim sisteminin
// paketleri, dış depo yalnız yöneticinin bilinçli seçimi (alpha.108). Önek: lane.

export const enVendorLanesScreens = {
    // ---- the lane dialog ----
    'lane.title.enable': 'Use {name} for {component}?',
    'lane.title.complete': 'Complete {name}’s setup?',
    'lane.checking': 'Reading what enabling it would write…',
    'lane.unknown': 'What enabling it would write could not be read, so it cannot be confirmed now. Nothing was changed. Try again.',
    'lane.none': 'This component has no vendor repository on this server.',
    'lane.intro.fresh': 'By default this server uses only its operating system’s own packages, which the operating system patches. {name} adds versions of {component} that the operating system does not ship: {name} builds and patches them, not the operating system. CelikPanel enables it only together with the three steps below, in one operation, so it is never left unconfined or without its security updates. No package is installed now.',
    'lane.intro.enabled': '{name} is enabled on this server. A vendor repository is safe only with all three steps below. CelikPanel writes the missing ones together and changes nothing that is already in place. No package is installed, removed or changed now.',
    'lane.step.repository.todo': 'Add the repository: the source line below, trusting only its pinned signing key.',
    'lane.step.repository.done': 'The repository is enabled.',
    'lane.step.confinement.todo': 'Confine it to {component}: with the rule below it provides only what the distribution does not ship and never replaces a distribution package, with CelikPanel or with your own apt upgrade.',
    'lane.step.confinement.done': 'Confined to {component}.',
    'lane.step.updates.todo': 'Add its origin {origin} to the operating system’s automatic updates, so its packages get their security updates like the distribution’s, also without CelikPanel.',
    'lane.step.updates.done': 'Its origin {origin} is among the operating system’s automatic updates.',
    'lane.owner.owner_rule': 'Your own preferences file sets the rule for this repository ({detail}). CelikPanel leaves it as it is.',
    'lane.owner.edited': 'The rule file {path} was changed on the server. CelikPanel keeps it as it is and does not check what it now allows.',
    'lane.intro.owner': 'You added {name} yourself ({detail}). CelikPanel adds no second source for it: two sources for one address make apt refuse every update. It offers only the steps below for your source; nothing is installed now.',
    'lane.owner.owner_source': 'Your own source for this repository: {detail}. CelikPanel keeps it as the only one and adds none.',
    'lane.owner.owner_config': 'Your own automatic-updates configuration already covers its origin {origin}.',
    'lane.blocked.automatic_off': 'The operating system’s automatic updates are off on this server, so this repository’s packages would not get their security updates. The server administrator turns them on first (sudo dpkg-reconfigure -plow unattended-upgrades, which asks one question in the terminal), then opens this again. Or add the repository yourself: CelikPanel then shows it as yours and not patched automatically.',
    'lane.blocked.automatic_not_installed': 'The operating system’s automatic updates (unattended-upgrades) are not installed, so this repository’s packages would not get their security updates. The server administrator installs them first (sudo apt-get install unattended-upgrades, then sudo dpkg-reconfigure -plow unattended-upgrades, which asks one question), then opens this again. Or add the repository yourself: CelikPanel then shows it as yours and not patched automatically.',
    'lane.blocked.drop_in_edited': '{path} was changed on the server, so CelikPanel does not rewrite it. The server administrator adds this line inside its Unattended-Upgrade::Origins-Pattern block: {detail} Then open this again.',
    'lane.blocked.duplicate_source': 'This address has two sources: CelikPanel’s {path} and your own {detail}. With different keys apt refuses every update. The server administrator removes one of them (sudo rm <file> and sudo apt-get update), then opens this again; CelikPanel removes neither.',
    'lane.blocked.origin_mismatch': 'The repository’s package list names its origin {detail}, not {origin}, so CelikPanel does not add it to the automatic updates. Nothing is written. The server administrator checks this repository’s source line on the server.',
    'lane.blocked.apt_config_unreadable': 'apt-config could not be read ({detail}), so whether the automatic updates cover it is not known. Nothing is written. Try again.',
    'lane.blocked.lists_unreadable': 'Its package list could not be read, so the confinement cannot be computed. The server administrator runs sudo apt-get update, then opens this again.',
    'lane.blocked.unknown': 'This step cannot be written now. Nothing is written. Try again.',
    'lane.kept': 'Installed from it and kept following it under the rule: {names}.',
    'lane.after': 'Customers never enable repositories. Disabling it later removes nothing: what was installed from it stays and is marked as no longer updated.',
    'lane.afterTools': 'phpMyAdmin, phpPgAdmin and webmail stay on the distribution’s PHP; the new versions are for the sites that choose them.',
    'lane.notReady.blocked': 'It cannot be written until the blocked step above is resolved.',
    'lane.notReady.nothing': 'Everything is in place; there is nothing to write.',
    'lane.planSha': 'Plan SHA-256 {sha}',
    'lane.confirm.enable': 'Enable with its confinement and updates',
    'lane.confirm.complete': 'Write the missing steps',
    'lane.done.enabled': '{name} is enabled, confined and among the automatic updates.',
    'lane.done.completed': '{name}’s missing steps were written.',
    'lane.done.notEffective': 'Written, but apt-config does not show the origin among the automatic updates yet. The security updates card says what it reads.',
    'lane.code.LANE_PLAN_CHANGED': 'Nothing was written: the plan changed after it was shown. It was read again; check it and confirm again.',
    'lane.code.LANE_BLOCKED': 'Nothing was written: a step is blocked. The plan says which, and what to do.',
    'lane.code.LANE_KEY_UNTRUSTED': 'Nothing was written: the repository’s signing key did not match the key CelikPanel pins for it. Try again later; if it stays so, the vendor has changed its key and CelikPanel needs an update that knows it.',
    'lane.code.LANE_ENABLE_FAILED': 'Nothing stays enabled: {reason} What this operation had written was taken back. Try again.',
    'lane.code.LANE_PARTIAL': 'Written: {completed}. The rest was not written: {reason} Press the button again to complete it.',
    'lane.code.LANE_INVALID': 'It was not written: {reason}',
    'lane.code.LANE_UNSUPPORTED': 'Vendor repositories need Debian or Ubuntu’s apt on this server.',
    'lane.code.LANE_RESULT_UNKNOWN': 'The answer did not arrive, so whether it was written is not known. The plan is read again and shows what the server holds; nothing is sent a second time.',
    'lane.stepName.repository': 'the repository',
    'lane.stepName.confinement': 'the confinement',
    'lane.stepName.updates': 'the automatic updates',
    // ---- the repository card ----
    'lane.default.title': '{component}: the operating system’s packages',
    'lane.default.body': '{component} comes from this server’s operating system, which patches it. Versions it does not ship come from {name}; that is the administrator’s informed choice, enabled only with its confinement and automatic updates. Customers cannot enable it.',
    'lane.default.left': 'Installed from {name} before it was disabled on {date}, and no longer updated: {names}. They keep running; remove them or enable {name} again.',
    'lane.default.enable': 'Use {name}…',
    'lane.card.missing': 'Missing for a safe vendor repository: {steps}. Complete… shows exactly what would be written.',
    'lane.card.blocked': 'A step cannot be written now. Complete… says why and what the server administrator does.',
    'lane.card.complete': 'Complete…',
    'lane.card.notConfined': 'Not confined: a plain apt upgrade can replace distribution packages with this repository’s builds.',
    'lane.card.refreshKey': 'Fetch its current key…',
    'lane.card.refreshKeyAsk': 'Fetch {name}’s current signing key from the vendor? CelikPanel accepts it only if it is the key it pins for this repository; the source line, the confinement and the automatic updates stay as they are.',
    'lane.card.refreshKeyDone': 'The key was read again.',
    'lane.key.expiredFile': 'The newest signature in its key file says it expired on {date}, but apt has not reported refusing this repository. When apt does, its packages stop being updated. Fetch its current key: the vendor usually publishes the key with a longer validity.',
    // ---- version lists ----
    'lane.locked.title': 'More versions from {repo}',
    'lane.locked.why': 'These versions come from {repo}, not from {os}: {repo} builds and patches them, not {os}. They stay locked until the server administrator enables {repo} with its confinement and automatic updates.',
    'lane.locked.whyNoOS': 'These versions come from {repo}, not from this server’s operating system: {repo} builds and patches them. They stay locked until the server administrator enables {repo} with its confinement and automatic updates.',
    'lane.locked.hint': 'Locked: comes from {repo}, not from the operating system',
    'lane.locked.enable': 'Use {repo}…',
    'lane.noLonger': 'no longer updated',
    'lane.noLongerHint': 'Installed from {repo}, which was disabled on {date}: nothing updates it any more. It keeps running. Remove it, or enable {repo} again.',
    'lane.tested': 'tested',
    'lane.testedHint': 'Tested with this CelikPanel release on {os}.',
    'lane.notTested': 'not tested',
    'lane.notTestedHint': 'Installable; not tested with this CelikPanel release on {os}.',
    'lane.tools.vendor': 'from {repo}: meant for sites',
    'lane.tools.vendorHint': 'This version comes from {repo}, not from the distribution. The panel’s own tools stay on the distribution’s PHP by default; choose it only if you have a reason.',
    // ---- the security updates card ----
    'lane.updates.fix': 'Complete {name}…',
    'lane.updates.fixHint': 'Writes what is missing — the confinement and the origin among the automatic updates — after showing it.',
} as const;

export type VendorLanesScreenKey = keyof typeof enVendorLanesScreens;
