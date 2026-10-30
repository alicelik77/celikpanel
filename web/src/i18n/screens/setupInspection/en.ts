// SCREEN catalogue part "setupInspection": the deletion check between two
// servers joined by code, set up by the backup over the setup link
// (setup.v5.inspection., and its waits setup.v5.wait.server_setup_peer_inspection_),
// split out of "setupLink" for its size budget.
// "setupInspection" ekran parcasi: iki sunucu arasindaki silme denetimi.

export const enSetupInspectionScreens = {
    "setup.v5.wait.server_setup_peer_inspection_unreachable": "This server is ready and is setting up, with the main DNS server, the check that confirms a deleted domain is gone from this backup too. The main DNS server could not be reached. Nothing to do if it is only briefly offline; otherwise check that its panel is running. This server tries again by itself.",
    "setup.v5.wait.server_setup_peer_inspection_main_agent": "This server is ready and is setting up the deletion check with the main DNS server, whose CelikPanel Agent did not answer. If it lasts, the main DNS server's owner checks it there with sudo systemctl status celikpanel-agent. This server tries again by itself.",
    "setup.v5.wait.server_setup_peer_inspection_agent": "This server is ready and is setting up the deletion check with the main DNS server, but this server's CelikPanel Agent did not answer. If it lasts, check it here with sudo systemctl status celikpanel-agent. This server tries again by itself.",
    "setup.v5.wait.server_setup_peer_inspection_main_pair": "This server is ready and is setting up the deletion check. The main DNS server has not finished its side of the DNS pair yet. Nothing to do; this server asks again by itself.",
    "setup.v5.wait.server_setup_peer_inspection_backup_pair": "This server is ready and is setting up the deletion check, but its own DNS pair is not recorded as set up yet. Nothing to do; this server asks again by itself.",
    "setup.v5.wait.server_setup_peer_inspection_ssh_port": "This server is ready, but the deletion check cannot be set up: it connects to this server's SSH on port 22, and SSH here listens on another port. You, the owner: let OpenSSH also listen on port 22 (see the line below), or choose \"Stop waiting for the other server\" on the main DNS server if you do not want the check. This server checks again by itself.",
    "setup.v5.wait.server_setup_peer_inspection_failed": "This server is ready, but a step of the deletion check setup was refused; nothing was overwritten. The line below names the server and the cause; that server's owner fixes it, and this server tries again by itself within a few minutes.",
    "setup.v5.wait.server_setup_peer_inspection_ssh_unreachable": "The deletion check is set up on both servers, but the main DNS server cannot connect to this server's SSH port 22. You, the owner: allow incoming TCP port 22 from the main DNS server in every firewall in front of this server (for example the hosting provider's). This server checks again by itself.",
    "setup.v5.inspection.title": "Deletion check between the two servers",
    "setup.v5.inspection.ready": "Set up: when a domain is deleted, the main DNS server ({main}) confirms by itself that the backup ({backup}) removed it too. Nothing to do. It is ordinary configuration on both servers and keeps working without the panel.",
    "setup.v5.inspection.existing": "A deletion check the server owner set up earlier was found and kept exactly as it is; nothing was changed. If deleted domains stay pending, compare it on both servers with the dns-peer-enroll owner tool's status commands.",
    "setup.v5.inspection.owner_conflict": "Not set up automatically: one of the servers already has inspection settings, files or SSH configuration the owner made or changed, and setup never overwrites them. Nothing was changed. Until this is resolved a deleted domain stays pending and later DNS changes wait behind it. The owner reviews them with the dns-peer-enroll owner tool (its status commands), then keeps or revokes them and completes the check with that tool.",
    "setup.v5.inspection.revoked": "The server owner turned the deletion check off on the backup, and setup does not turn it back on. A deleted domain stays pending until the check is set up again with the dns-peer-enroll owner tool; DNS keeps answering meanwhile.",
    "setup.v5.inspection.inspector_unavailable": "Not set up automatically: the backup has no packaged inspector program (an installation updated from an earlier release). A deleted domain stays pending until the check is set up with the dns-peer-enroll owner tool from the verified release archive.",
    "setup.v5.inspection.unsupported": "Not set up automatically: one of the two panels does not offer this yet. Update the older panel from its own update screen; until then a deleted domain stays pending unless the check is set up with the dns-peer-enroll owner tool.",
    "setup.v5.inspection.link_ended": "Not set up: the main DNS server stopped waiting for the backup before the check was finished. A deleted domain stays pending until the check is set up with the dns-peer-enroll owner tool on both servers.",
    "setup.v5.inspection.working": "Being set up between the two servers. Nothing to do.",
    "setup.v5.inspection.unreachable": "Waiting: the backup could not reach the main DNS server ({main}). It tries again by itself.",
    "setup.v5.inspection.main_agent": "Waiting: the main DNS server's CelikPanel Agent did not answer. It is asked again by itself.",
    "setup.v5.inspection.agent": "Waiting: the backup's CelikPanel Agent did not answer. It is asked again by itself.",
    "setup.v5.inspection.main_pair": "Waiting for the main DNS server ({main}) to finish its side of the DNS pair. Nothing to do.",
    "setup.v5.inspection.backup_pair": "Waiting for the backup's DNS pair to be recorded as set up. Nothing to do.",
    "setup.v5.inspection.ssh_port": "Waiting for the owner: the check connects to the backup's ({backup}) SSH on port 22, and SSH there listens on port {ports}. On the backup add \"Port 22\" next to the current Port line in /etc/ssh/sshd_config, run sudo sshd -t, then reload OpenSSH; CelikPanel's firewall opens the ports OpenSSH listens on. It is checked again by itself.",
    "setup.v5.inspection.failed": "Waiting for the owner: a step on the {side} server was refused; nothing was overwritten and staged files are kept. That server's owner reads the reason there with sudo journalctl -u celikpanel-agent | grep \"peer inspection\" and fixes it; the step is tried again by itself within a few minutes.",
    "setup.v5.inspection.ssh_unreachable": "Waiting for the owner: the main DNS server ({main}) could not connect to the backup's ({backup}) SSH port 22. Allow incoming TCP port 22 from {main} in every firewall in front of the backup (for example the hosting provider's). It is checked again by itself.",
    "setup.v5.inspection.ssh_host_key_mismatch": "Waiting for the owner: the main DNS server reached SSH port 22 at {backup}, but it answered with another server key than the backup's. Check that {backup} is really the backup's address and that nothing in between answers for it. It is checked again by itself.",
    "setup.v5.inspection.ssh_auth_refused": "Waiting for the owner: the backup's SSH at {backup} refused the check's key. On the backup, check it with sudo /opt/celikpanel/dns-owner-tools/dns-peer-enroll secondary-status (add --engine pdns for PowerDNS) and the SSH log. It is checked again by itself.",
    "setup.v5.inspection.ssh_credential": "Waiting: the main DNS server could not read its own key for the check. Its owner checks the CelikPanel Agent log there (sudo journalctl -u celikpanel-agent | grep \"peer inspection\"). It is checked again by itself.",
    "setup.v5.inspection.side.main": "main DNS",
    "setup.v5.inspection.side.backup": "backup DNS",
} as const;

export type SetupInspectionScreenKey = keyof typeof enSetupInspectionScreens;
