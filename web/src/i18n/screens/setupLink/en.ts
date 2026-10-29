// SCREEN catalogue part "setupLink": the waits and verified states of the
// setup link between two servers (names, binding, DKIM, publishing, the
// link's end, completion), split out of "setupJoin" (v0.1.0-alpha.114,
// prefixes setup.v5.wait., setup.v5.check.).
// "setupLink" ekran parcasi: iki sunucu arasindaki kurulum baglantisinin
// beklemeleri ve dogrulanmis durumlari.

export const enSetupLinkScreens = {
    "setup.v5.wait.server_setup_peer_join_waiting": "Waiting for the second server to join. Create a join code on this screen and enter it on the second server.",
    "setup.v5.wait.server_setup_peer_names_waiting": "The second server joined. Waiting for its setup to start, which sends its names here.",
    "setup.v5.wait.server_setup_peer_join_changed": "The answers changed after the second server joined (on this server or on the second one). Create a new join code here and enter it on the second server; setup then continues by itself.",
    "setup.v5.wait.server_setup_peer_join_reopen_required": "The second server's answers changed after this setup run already used them. Reopen this server's plan and start it again, then create a new join code here and enter it on the second server.",
    "setup.v5.wait.server_setup_peer_names_clash": "The second server's names cannot be added to the zone (see the details). Change them in the second server's plan, or the conflicting record in Domains.",
    "setup.v5.wait.server_setup_peer_names_outside_domain": "The second server's names are not under the main domain. Change them in the second server's plan.",
    "setup.v5.wait.server_setup_peer_names_invalid": "The second server sent names this server cannot use. Review the second server's plan again.",
    "setup.v5.wait.server_setup_peer_mail_both": "Both servers would receive the main domain's mail. Choose mail on one server.",
    "setup.v5.wait.server_setup_peer_names_unreachable": "The first server cannot be reached to send this server's names. Check that it is running; setup asks again by itself.",
    "setup.v5.wait.server_setup_peer_names_unknown": "The first server's answer was not received. Setup sends the same names again; nothing is done twice.",
    "setup.v5.wait.server_setup_dns_publisher_waiting": "Waiting for the main DNS server to accept this server's publishing connection. It does once its DNS pair is ready and its nameservers are public; setup asks again by itself.",
    "setup.v5.wait.server_setup_dns_publisher_local_waiting": "This backup DNS server waits until it serves the main DNS server's list and both nameserver names are public. Setup asks again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_zone_waiting": "The main DNS server has not prepared the main domain's zone yet. Setup sends this server's DKIM record again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_publishing": "The main DNS server saved this server's DKIM record and is publishing it. Setup checks again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_unreachable": "The main DNS server cannot be reached to send this server's DKIM record. Setup asks again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_unknown": "The main DNS server's answer was not received. Setup sends the same record again; nothing is done twice.",
    "setup.v5.wait.server_setup_join_certificate_unavailable": "This panel does not serve a valid certificate of its own on its IP address (its self-signed panel certificate is missing, damaged or expired), and the other server checks exactly that key, so it cannot be paired by code. The server owner restores the panel's own certificate (panel.crt and panel.key in the panel's tls folder, for example from a backup) and restarts the panel; then try again.",
    "setup.v5.wait.server_setup_completion_plan_changed": "This server's saved answers no longer match the plan this setup runs, so setup is not marked complete. Review the plan again on this screen; setup checks again by itself.",
    "setup.v5.wait.server_setup_completion_unknown": "Whether setup is complete could not be checked: its plan or the second server's binding could not be read. Nothing to do here; setup checks again by itself.",
    "setup.v5.wait.server_setup_peer_link_revoked": "The setup link between the two servers has ended (a setup finished or was run again), so nothing more is sent over it. Create a new join code on the first server and enter it on the second server, in the first step of its setup; setup continues when it is started again.",
    "setup.v5.wait.server_setup_peer_link_missing": "This server is no longer joined to the first server. Join it again from the first step of setup, then review the plan.",
    "setup.v5.wait.server_setup_peer_names_bound": "The first server already uses this server's earlier answers. Create a new join code on the first server, enter it here in the join step, then review and start this plan again; setup then continues with these answers.",
    "setup.v5.wait.server_setup_distribution_updates_changed": "New distribution updates appeared since the review, so nothing was installed. You, the owner: open the review and start setup again; it lists them for you to confirm.",
    "setup.v5.wait.server_setup_distribution_updates_unread": "CelikPanel could not read which distribution packages this component's install would also update, so nothing was installed. Nobody needs to act yet; setup asks again by itself.",
    "setup.v5.wait.server_setup_peer_names_final": "The first server's setup finished with this server's earlier answers. Changing them after a finished setup is not available in this release; keep the earlier answers in this plan.",
    "setup.v5.wait.server_setup_peer_version_incompatible": "The two panels run versions that cannot work together. You update the older panel from its own update screen (Settings → Updates); setup then asks again by itself.",
    "setup.v5.wait.server_setup_dns_publisher_refused": "The main DNS server refused this server's publishing connection: its enrollment code expired, was withdrawn or was already used. On the main DNS server create a new join code, join this server again, then review this plan again.",
    "setup.v5.wait.server_setup_peer_dkim_conflict": "The main DNS server already has a different DKIM record at this name that setup did not write. Change or remove it there in Domains, then review this server's plan again.",
    "setup.v5.wait.server_setup_peer_dkim_not_running": "The main DNS server's setup is not running. Nothing to do here if it is about to start; otherwise start or resume setup on the main DNS server. This server sends its DKIM record again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_not_bound": "The main DNS server's setup has not taken this server in yet; it does once it has this server's names (check its join step if it waits there). Setup sends the DKIM record again by itself.",
    "setup.v5.check.peerDKIM": "Waiting for the other server's DKIM record: it is sent here once that server's mail is set up. Nothing to do here; setup checks again by itself.",
    "setup.v5.wait.other": "Waiting for the other server ({code}). Setup asks again by itself.",
} as const;

export type SetupLinkScreenKey = keyof typeof enSetupLinkScreens;
