// SCREEN catalogue part "nodeSandbox" (alpha.127): Node.js sites in the site
// sandbox - the administrator's card under Components → Node.js (the move of
// sites an earlier release wrote) and the site's own line and install step on
// its Hosting tab (nodesandbox.). Each refusal says what is wrong, who acts and
// the next action (D-024).
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "nodeSandbox" ekran parçası: site korumasındaki Node.js siteleri.

export const enNodeSandboxScreens = {
    'nodesandbox.title': 'Node.js sites in the site sandbox',
    'nodesandbox.full': 'This server enforces the full site sandbox (systemd {systemd}).',
    'nodesandbox.reduced': 'This server enforces a reduced site sandbox (systemd {systemd}). Not enforced here:',
    'nodesandbox.inside': '{count} Node.js sites run in the site sandbox: each writes only to its own folder, sees only its own Node.js version, opens only its own port and cannot send mail through the local mail server.',
    'nodesandbox.needs': '{count} Node.js sites still run in an earlier release’s protection ({running} running). Nothing changes until you move them.',
    'nodesandbox.move': 'Move {count} sites into the sandbox',
    'nodesandbox.confirmTitle': 'Move {count} Node.js sites into the site sandbox?',
    'nodesandbox.confirmBody': 'One site at a time: each running application restarts once in the sandbox and must answer on its port; if it does not, or the sandbox cannot be proved, its earlier unit is put back and started again. Stopped applications get the new unit and stay stopped. Units changed on the server are kept.',
    'nodesandbox.confirm': 'Move them',
    'nodesandbox.progress': 'Moving Node.js sites into the site sandbox',
    'nodesandbox.step': 'Moving one site at a time and proving each',
    'nodesandbox.done': '{moved} running sites moved and proved; {rewritten} stopped sites got the new unit.',
    'nodesandbox.putBack': 'Kept in the earlier protection because the proof failed; they run again on their earlier unit: {sites}. The site’s owner makes the application listen only on its own port and write only inside its own folder; then move again.',
    'nodesandbox.skipped': 'Not moved, nothing changed: {sites}.',
    'nodesandbox.edited': 'Changed on the server, kept as they are and not moved: {sites}. Move your own settings into the unit’s drop-in folder (or put CelikPanel’s text back on the site’s configuration page) to move them.',
    'nodesandbox.shared': 'Run as the shared web account, so they cannot be moved yet: {sites}. Use “Run under the site’s own user” on each site’s Hosting tab first.',
    'nodesandbox.unknown': 'The site sandbox could not be read from the server. It is read again when this page is opened.',
    'nodesandbox.busy': 'Node.js sites are being moved now. This page updates by itself.',
    'nodesandbox.lost': 'The answer was lost. Press the button again: the same request returns the first answer and moves nothing twice.',
    'nodesandbox.why.not_running': 'did not run in the sandbox',
    'nodesandbox.why.not_listening': 'did not answer on its port',
    'nodesandbox.why.listens_elsewhere': 'listens on another port or a public address',
    'nodesandbox.why.port_taken': 'another program holds its port',
    'nodesandbox.why.property_mismatch': 'systemd did not apply the sandbox (a drop-in?)',
    'nodesandbox.why.unit_edited': 'its unit was changed on the server',
    'nodesandbox.why.shared_account': 'runs as the shared web account',
    'nodesandbox.why.app_folder_not_real': 'its folder is a link or contains one',
    'nodesandbox.why.app_folder_not_owned': 'its folder belongs to another account',
    'nodesandbox.why.agent_unreachable': 'the server did not answer',
    'nodesandbox.why.other': 'see the audit log',
    'nodesandbox.site.earlier': 'This application runs in an earlier release’s protection. The administrator moves it into the site sandbox under Components → Node.js; it then restarts once.',
    'nodesandbox.site.inside': 'Runs in the site sandbox: it writes only to this site’s folder and opens only its own port.',
    'nodesandbox.install': 'Install packages and build',
    'nodesandbox.installTitle': 'Install the packages of {site}?',
    'nodesandbox.installBody': 'npm installs what package.json names (exactly what package-lock.json names, when it is there), then runs the project’s build script if it has one. It runs as the site’s own account in the site sandbox. The running application is not changed: restart it afterwards to use the new packages.',
    'nodesandbox.installConfirm': 'Install',
    'nodesandbox.installProgress': 'Installing the packages of {site}',
    'nodesandbox.installStep': 'npm install and build in the site sandbox',
    'nodesandbox.installDone': 'Packages installed. Restart the application to use them.',
    'nodesandbox.reason.install_failed': 'npm ended with an error; its last lines are shown above. Correct the project, then install again.',
    'nodesandbox.reason.install_lost': 'The install step is no longer on the server (the server restarted while it ran). Install again.',
    'nodesandbox.reason.earlier_release': 'Packages are installed only in the site sandbox, and this application still runs in an earlier release’s protection. The administrator moves it under Components → Node.js; then install again.',
    'nodesandbox.reason.package_json_missing': 'There is no package.json in the site’s folder. Upload the project with the file manager or SFTP, then install again.',
    'nodesandbox.reason.package_json_not_file': 'package.json in the site’s folder is a link or a folder. Put the real file there, then install again.',
    'nodesandbox.reason.node_missing': 'The site’s Node.js version is not installed on this server. The administrator installs it under Components → Node.js.',
    'nodesandbox.reason.install_running': 'An install step of this site is still running. It is followed here until it ends.',
    'nodesandbox.reason.app_folder_not_real': 'The site’s folder is a link or contains one. Put the real folder back, then install again.',
    'nodesandbox.reason.app_folder_not_owned': 'The site’s folder belongs to another account. The administrator gives it back to the site’s account, then install again.',
    'nodesandbox.reason.unknown': 'The server did not say how the install step ended ({detail}). Read the state again later; nothing was started twice.',
} as const;

export type NodeSandboxScreenKey = keyof typeof enNodeSandboxScreens;
