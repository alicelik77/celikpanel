// SCREEN catalogue part "removalRule" (alpha.123, D-045): the component
// dependency rule on screen - the removal window's three groups (what must be
// moved or removed first, what is removed together, what stops), the
// server-side removal's progress, the install window's "needs first" and the
// refusal code REMOVAL_NEEDS_TOGETHER. Key prefix kept here: rmrule.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "removalRule" ekran parçası: bileşen bağımlılık kuralı ekranda.

export const enRemovalRuleScreens = {
    'rmrule.firstTitle': 'Must be moved or removed first',
    'rmrule.togetherTitle': 'Will be removed together',
    'rmrule.stopsTitle': 'What stops',
    'rmrule.next': 'Who acts: you. Next: move or remove each item above with its link, then open this window again; {name} can be removed when nothing is left here.',
    'rmrule.more': 'and {n} more',
    'rmrule.removeFirst': 'Remove {name} first',
    'rmrule.kind.component': '{name} needs it and is used itself, so it is not removed together.',
    'rmrule.kind.sites': '{count} site(s) are served by it. Remove a site, or make it DNS only, on its page.',
    'rmrule.kind.apache_sites': '{count} site(s) run on Apache. Move each to nginx on its page, or remove it.',
    'rmrule.kind.php_tools': '{count} PHP tool(s) run on it as well. Open each and move it to another PHP version in its Manage window.',
    'rmrule.kind.php_sites': '{count} site(s) run on it. Open each site and change its PHP version.',
    'rmrule.kind.node_sites': '{count} site(s) run on it. Open each site and change its Node.js version.',
    'rmrule.kind.node_service_apps': '{count} app(s) run on it (n8n, for example). Remove the app on its site’s page first.',
    'rmrule.kind.python_apps': '{count} Python app(s) run on it. Remove them on their site’s page first.',
    'rmrule.kind.databases': '{count} database(s) are stored in it. Move or delete them on the Databases page first.',
    'rmrule.kind.mail_addresses': '{count} mailbox(es) or forwarding(s) use it. Remove them on the domain’s Mail tab first.',
    'rmrule.kind.dns_zones': '{count} DNS zone(s) are served by it.',
    'rmrule.kind.certificates': '{count} certificate(s) are renewed with it. Without it they expire within 90 days and HTTPS stops working.',
    'rmrule.kind.panel_firewall': 'The panel firewall runs on it. Turn the firewall off on the Security page first.',
    'rmrule.kind.panel_firewall_unknown': 'The panel firewall’s state could not be read now, so it is not known whether the firewall runs on it. Nothing was changed. Try again in a moment.',
    'rmrule.kind.vpn_peers': '{count} VPN peer(s) connect through it. Remove them on the VPN page first.',
    'rmrule.kind.malware_quarantine': '{count} file(s) are held in malware quarantine. Restore or delete them in the site’s Malware scan section first.',
    'rmrule.togetherHint': 'Nothing else uses them, and they cannot work without {name}. They are removed first, on the server.',
    'rmrule.togetherKeep': 'Kept: then {name} cannot be removed now, because these would stop working. Tick the box again to remove them together.',
    'rmrule.setting.malware_schedule': 'the weekly scheduled malware scan (turned off)',
    'rmrule.stop.webmail': 'Webmail stops for {count} mailbox(es). Mail keeps arriving and being sent.',
    'rmrule.stop.spam_filtering': 'Spam filtering stops for {count} mail domain(s). Mail is still delivered, unfiltered.',
    'rmrule.stop.ban_protection': 'Repeated failed logins are no longer banned. Logins and the firewall keep working.',
    'rmrule.stop.malware_scans': 'Malware scans stop. Files already in quarantine stay where they are.',
    'rmrule.stop.ftp': '{count} hosting account(s) lose FTP. SFTP keeps working.',
    'rmrule.stop.monitoring': 'The live monitoring dashboard stops. Sites and services keep working.',
    'rmrule.stop.unknown_use': 'CelikPanel does not record which sites use it, so it cannot say who is affected. Anything on this server that uses it stops.',
    'rmrule.stop.toolchain': 'The compiler is no longer on this server. Nothing hosted uses it.',
    'rmrule.stop.database_tool': 'The web page for managing databases closes. The databases stay.',
    'rmrule.nothingStops': 'Nothing on this server requires it.',
    'rmrule.withdraws': 'Offered to customers: {count} entr(ies) in {sets} plan(s) or account(s) are withdrawn with it. Sites already using it are listed above, so none breaks.',
    'rmrule.unitsUnknown': 'Which apps run on this version could not be read now. The server checks again when you press Remove and refuses if one still runs on it.',
    'rmrule.done': 'Removed: {names}.',
    'rmrule.removedSoFar': 'Already removed: {names}. Press Remove again to continue; what is already gone is skipped.',
    'rmrule.install.title': '{name} needs other components first',
    'rmrule.install.why': '{name} cannot work without these. Install them first; nothing is installed until you press a button.',
    'rmrule.install.role': 'Any one of these fills the role of {role}.',
    'rmrule.install.none': 'None can be installed here now. Open its row on Components for the reason.',
    'rmrule.install.first': 'Install {name} first',
    'rmrule.install.next': 'Who acts: you. Next: install what is missing; then press Install on {name} again.',
    'rmrule.emergencyTitle': 'What requires it (the emergency removal goes ahead; these stop or lose it)',
    'dashboard.audit.action.service.uninstall.together': 'removed {target} together with what only it used',
    'dashboard.audit.action.service.uninstall.together.failed': 'could not finish removing {target} together',
    'dashboard.audit.action.service.uninstall.together.recovered': 'finished removing {target} together after a restart',
    'dashboard.audit.action.service.uninstall.together.recovered.failed': 'could not finish removing {target} together after a restart',
    'rmrule.needs': '{name} needs {needs}, which is not installed, so it cannot work. Who acts: you. Next: install the {needs} with the button here; {name} keeps its settings.',
    'err.REMOVAL_NEEDS_TOGETHER': 'Some components can only be removed together with it: nothing else uses them and they cannot work without it. Nothing was removed. Who acts: you. Next: leave them ticked in the removal window and press Remove again.',
} as const;

export type RemovalRuleScreenKey = keyof typeof enRemovalRuleScreens;
