// SCREEN catalogue part "setupPublishing": the setup link's waits about the
// DNS publisher and the mail signing (DKIM) names (setup.v5.wait.server_setup_dns_publisher_,
// setup.v5.wait.server_setup_peer_dkim_, setup.v5.check.peerDKIM), split out of
// "setupLink" for its size budget (alpha.114).
// "setupPublishing" ekran parcasi: DNS yayinlayici ve DKIM beklemeleri.

export const enSetupPublishingScreens = {
    "setup.v5.wait.server_setup_dns_publisher_waiting": "Waiting for the main DNS server to accept this server's publishing connection. It does once its DNS pair is ready and its nameservers are public; setup asks again by itself.",
    "setup.v5.wait.server_setup_dns_publisher_local_waiting": "This backup DNS server waits until it serves the main DNS server's list and both nameserver names are public. Setup asks again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_zone_waiting": "The main DNS server has not prepared the main domain's zone yet. Setup sends this server's DKIM record again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_publishing": "The main DNS server saved this server's DKIM record and is publishing it. Setup checks again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_unreachable": "The main DNS server cannot be reached to send this server's DKIM record. Setup asks again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_unknown": "The main DNS server's answer was not received. Setup sends the same record again; nothing is done twice.",
    "setup.v5.wait.server_setup_peer_dkim_removed": "The DKIM record setup wrote for this server was removed on the main DNS server by its owner, and an owner's deletion is kept. You, the owner: add the record back yourself in Domains on the main DNS server, or keep it removed; then mail from this server is not signed for the main domain.",
    "setup.v5.wait.server_setup_dns_publisher_refused": "The main DNS server refused this server's publishing connection: its enrollment code expired, was withdrawn or was already used. On the main DNS server create a new join code, join this server again, then review this plan again.",
    "setup.v5.wait.server_setup_peer_dkim_conflict": "The main DNS server already has a different DKIM record at this name that setup did not write. Change or remove it there in Domains, then review this server's plan again.",
    "setup.v5.wait.server_setup_peer_dkim_not_running": "The main DNS server's setup is not running. Nothing to do here if it is about to start; otherwise start or resume setup on the main DNS server. This server sends its DKIM record again by itself.",
    "setup.v5.wait.server_setup_peer_dkim_not_bound": "The main DNS server's setup has not taken this server in yet; it does once it has this server's names (check its join step if it waits there). Setup sends the DKIM record again by itself.",
    "setup.v5.check.peerDKIM": "Waiting for the paired server's DKIM record: it is sent here once that server's mail is set up. Nothing to do here; setup checks again by itself.",
    "setup.v5.wait.server_setup_dns_publisher_peer_busy": "The main DNS server {server} is busy, restarting or finishing its own setup, so it could not take this server's publishing connection yet. Nobody needs to act: setup asks again by itself.",
    "setup.v5.wait.server_setup_dns_publisher_peer_license": "The main DNS server {server} answered that its panel license is not active or could not be checked, so it takes no connection now. Its owner activates or renews the license on {server}; setup then continues by itself.",
    "setup.v5.wait.server_setup_dns_publisher_peer_unexplained": "The main DNS server {server} did not accept this server's publishing connection and did not say why. This happens while it restarts or finishes its own setup. Nobody needs to act yet: setup asks again by itself, and after 30 minutes it says what to check.",
    "setup.v5.wait.server_setup_dns_publisher_unconfirmed": "For 30 minutes the main DNS server {server} refused this server's publishing connection without saying why. You, the owner: open the panel of {server} and check that its own setup has finished. If it has, create a new join code there, join this server again with it, then review this plan again.",
} as const;

export type SetupPublishingScreenKey = keyof typeof enSetupPublishingScreens;
