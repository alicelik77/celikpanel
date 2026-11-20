// SCREEN catalogue part "serviceAppsAdmin" (alpha.141): the administrator's
// Catalog management of AI and automation apps (offering, limits, security
// hold) and the site sandbox panel, split out of "serviceApps" when the
// sentences built from the server's runtime facts filled it
// (docs/AI-APPS.md). Key prefixes kept here: svcapps.admin., svcapps.sandbox.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "serviceAppsAdmin" ekran parçası: yöneticinin yapay zekâ uygulamaları
// kataloğu ve site korumalı alanı paneli; "serviceApps"tan ayrıldı.

export const enServiceAppsAdminScreens = {
    'svcapps.sandbox.full': 'This server enforces the full site sandbox (systemd {systemd}).',
    'svcapps.sandbox.reduced': 'This server enforces a reduced site sandbox (systemd {systemd}). Missing:',
    'svcapps.sandbox.gap.systemd_old': 'systemd is older than 249: no port-binding rule. The administrator moves to a supported system.',
    'svcapps.sandbox.gap.systemd_unknown': 'The systemd version could not be read; the level is not known.',
    'svcapps.sandbox.gap.bpf_missing': 'systemd lacks BPF: the port-binding rule and the cloud-metadata block do not apply. An app that opens a public port is still stopped.',
    'svcapps.sandbox.gap.cgroup_v2_missing': 'No cgroup v2: the binding rules and the mail block cannot attach.',
    'svcapps.sandbox.gap.nft_cgroup_missing': 'nftables cannot match a program’s group: no mail block, so apps can send mail through this server.',
    'svcapps.sandbox.smtp': 'Mail ports 25, 465 and 587 are closed to apps (the local mail server too); {count} attempts refused.',
    'svcapps.sandbox.smtpIdle': 'The mail block loads when the first app starts.',
    'svcapps.sandbox.smtpNotCovered': 'Not covered: port 2525 and mail APIs.',
    'svcapps.sandbox.egressKept': 'egress.nft was changed on the server; CelikPanel keeps it and puts its own text beside it (.celikpanel-pending). If your file does not load, apps do not start.',
    'svcapps.sandbox.edited': '{count} apps have a service file changed on the server; they are not changed. Move the changes to the drop-in folder, then apply again.',
    'svcapps.sandbox.needs': '{count} apps run in an earlier protection ({running} running). Applying restarts each running app once and proves its protection; stopped apps stay stopped. Who acts: you.',
    'svcapps.sandbox.apply': 'Apply the new sandbox to {count} apps',
    'svcapps.sandbox.busy': 'The new sandbox is being applied. Nothing to do; this page updates by itself.',
    'svcapps.sandbox.result': 'Restarting: {started}. Kept stopped: {rewritten}. Left alone: {skipped}.',
    'svcapps.sandbox.skip.unit_edited': 'service file changed',
    'svcapps.sandbox.skip.update_first': 'update it first',
    'svcapps.sandbox.skip.busy': 'another operation runs',
    'svcapps.sandbox.skip.agent_unreachable': 'the server did not answer',
    'svcapps.sandbox.lost': 'The answer was lost; the result is not known. Press again: the same request is answered, nothing starts twice.',
    'svcapps.sandbox.unknown': 'The server did not answer; the sandbox state is not known. Nothing was changed.',
    'svcapps.admin.title': 'AI and automation apps',
    'svcapps.admin.desc': 'Third-party apps customers can run on their site as a service. Installed is not offered: nothing is open until you offer it after reading what it can do. Resellers and plans can only narrow what you offer.',
    'svcapps.admin.offered': 'Offered',
    'svcapps.admin.notOffered': 'Not offered',
    'svcapps.admin.offerStale': 'This release changed the app’s version or risk summary. Read it again and offer it again; until then nobody can install or update it. What runs keeps running.',
    'svcapps.admin.offer': 'Offer to customers',
    'svcapps.admin.withdraw': 'Stop offering',
    'svcapps.admin.withdrawNote': 'Nobody can install or update it any more; apps that run keep running.',
    'svcapps.admin.hold': 'Security hold',
    'svcapps.admin.holdNote': 'Stops every install of this app now and keeps them from starting until you lift the hold. Data is never removed.',
    'svcapps.admin.holdReason': 'Reason (for example the CVE)',
    'svcapps.admin.release': 'Lift the hold',
    'svcapps.admin.releaseNote': 'Nothing starts by itself; each owner starts their app again.',
    'svcapps.admin.held': 'On security hold: {reason}',
    'svcapps.admin.installs': '{total} installed, {running} running',
    'svcapps.admin.limits': 'Limits for every install',
    'svcapps.admin.memory': 'Memory (MB)',
    'svcapps.admin.cpu': 'CPU (% of one core)',
    'svcapps.admin.tasks': 'Processes',
    'svcapps.admin.runtime.panel': 'Uses the panel’s Node.js runtime.',
    'svcapps.admin.runtime.distribution': 'Uses the operating system’s own runtime.',
    'svcapps.admin.held.stopped': 'Stopped: {list}',
    'svcapps.admin.held.notStopped': 'Could not be stopped: {list}. Check these on the server.',
    'svcapps.admin.deferredTitle': 'Not offered in this release',
};

export type ServiceAppsAdminScreenKey = keyof typeof enServiceAppsAdminScreens;
