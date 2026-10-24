// SCREEN catalogue part "lifecycle": versions, use and security of every
// component (v0.1.0-alpha.107, D-038): the repository card, security
// updates, restarts and reboots, the dated support table, the supported
// operating-system releases, webmail and Node.js updates, an application's
// account. Key prefix kept here: life.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "lifecycle": her bileşenin sürümü, kullanımı ve
// güvenliği (alpha.107, D-038). Önek: life.

export const enLifecycleScreens = {
    // ---- the repository card (D-038 §12, §1/§7) ----
    'life.repo.title': 'Package repository: {name}',
    'life.repo.checking': 'Reading the repository…',
    'life.repo.unknown': 'CelikPanel could not read this repository, so nothing about it is shown. Nothing was changed. Try again.',
    'life.repo.state.confined': 'Confined to {component}: it adds versions the distribution does not ship and never replaces a distribution package, with CelikPanel or with your own apt upgrade.',
    'life.repo.state.edited': 'The rule file {path} that confines this repository was changed on the server. CelikPanel keeps it as it is and does not check what it now allows.',
    'life.repo.state.owner_rule': 'Your own preferences file sets the rule for this repository ({files}). CelikPanel leaves it as it is and writes no rule of its own.',
    'life.repo.state.not_written': 'Not confined yet: a plain apt upgrade could replace distribution packages with this repository’s builds. Confining it changes no installed package.',
    'life.repo.state.unknown': 'Whether this repository is confined to its component is not known: this server’s Agent does not report it.',
    'life.repo.reason.lists_unreadable': 'It could not be confined because apt has not downloaded its package list yet. The server administrator runs sudo apt-get update, then presses Confine again.',
    'life.repo.reason.write_failed': 'It could not be confined: the rule file could not be written ({detail}). Nothing was changed. Press Confine to try again.',
    'life.repo.confine': 'Confine',
    'life.repo.confining': 'Confining…',
    'life.repo.confineFailed': 'The repository could not be confined. Nothing was changed. Try again; the card says why when it is known.',
    'life.repo.kept': 'Installed from this repository before it was confined, and still following it, so they keep getting its updates ({count}): {names}.',
    'life.repo.keptMore': '{names} and {more} more',
    'life.repo.writtenBy.enable': 'Confined when it was enabled, {when}.',
    'life.repo.writtenBy.panel_upgrade': 'Confined by CelikPanel after its update, {when}.',
    'life.repo.enabledBy': 'Enabled by {who}, {when}.',
    'life.repo.enabledAt': 'Enabled {when}.',
    'life.repo.enabledUnknown': 'When it was enabled is not recorded (before this release).',
    'life.repo.key': 'Signing key {fingerprint}',
    'life.repo.key.valid': 'does not expire',
    'life.repo.key.validUntil': 'valid until {date}',
    'life.repo.key.expiring': 'Its signing key expires on {date}. Before then the server administrator enables the repository again on this page: that fetches the key the repository signs with now. Until it is renewed, apt keeps working.',
    'life.repo.key.expired': 'Its signing key expired on {date}: apt no longer accepts this repository’s updates, so its packages are not updated. The server administrator enables the repository again on this page to fetch its current key.',
    'life.repo.key.unknown': 'The signing key’s validity could not be read.',
    'life.repo.lists.fresh': 'Package list from {date}.',
    'life.repo.lists.missing': 'apt has no package list from this repository yet: until sudo apt-get update reaches it, its versions and updates are not known.',
    'life.repo.lists.expired': 'The repository’s package list expired ({date}): apt treats it as out of date and stops taking updates from it. The server administrator runs sudo apt-get update; if the error stays, the repository itself is stale.',
    'life.repo.aptErrors': 'apt reported this about the repository the last time CelikPanel refreshed the package lists ({when}):',
    'life.repo.moving.title': 'Packages that follow the default version',
    'life.repo.moving.item': '{package} follows the distribution’s default version; today it brings {exact}.',
    'life.repo.moving.itemNoExact': '{package} follows the distribution’s default version.',
    'life.repo.moving.confined': 'While this repository is confined, these stay on the distribution’s version. If the rule is removed, a plain apt upgrade could move them to another version.',
    'life.repo.moving.notConfined': 'This repository is not confined, so a plain apt upgrade could move these to another version. Confine it above.',
    'life.repo.moving.next': 'Optional, on the server: keep the exact version and drop the moving package with {command}. CelikPanel does not remove it.',
    'life.repo.outsideUpdates': 'The operating system’s automatic updates do not cover this repository unless it is added to them: see Security → Security updates.',
} as const;

export type LifecycleScreenKey = keyof typeof enLifecycleScreens;
