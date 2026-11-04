---
id: dns-servers
title: DNS and peer servers
roles: [admin]
topics: [dns.servers, dns.infrastructure#infrastructure-zone]
keywords: [dns, nameserver, ns1, ns2, bind, powerdns, secondary, backup, glue, registrar, zone transfer]
---

**DNS and peer servers** shows which server answers for your names and how the internet sees them. DNS runs on BIND or PowerDNS and keeps answering when the panel is stopped.

## Reading the page {#reading}

The top card reads from the internet and says **Why**, **Who acts**, **Next step** and **Then**. **Check now** asks again and changes nothing.

**Your DNS servers** shows each server as **Main DNS server** (changes are made here), **Backup DNS server** (keeps a copy and answers too) or **Single DNS server**. "Could not be asked just now" means unknown, not broken: check again in a few minutes.

## Two servers {#pair}

The main DNS server sends its domains to the backup by standard zone transfer, and the backup serves them on its own; neither needs the other's panel. A backup joins during server setup with a join code (see [First setup](guide:server-setup#join)). A PowerDNS server set up alone is paired under **Advanced**; a BIND server already running alone cannot gain a backup this way.

## At your registrar {#registrar}

"One task at your registrar" means the internet does not yet see your nameservers or their IP records (glue). Enter the values the card shows at your registrar. The panel notices by itself; registrars can take hours to publish.

## Changes made outside the panel {#outside}

"Changed outside CelikPanel" means the DNS was changed directly on the server, for example with `rndc`. The panel keeps your change. While it stays, the panel publishes no DNS change on this server; handing DNS back to the panel after such an edit is not available yet.

## Advanced {#advanced}

**Advanced** holds the DNS software (**Authoritative DNS server**), the pairing, and **Connections to DNS servers** for panels that publish records here. **Revoke DNS changes** ends such a connection; existing zones keep serving.

## The infrastructure zone {#infrastructure-zone}

Setup creates a zone for the panel's own names: panel addresses, mail hosts and nameservers. It is marked **Infrastructure zone · DNS only** and has no website or mailboxes. Records marked **Managed by server setup** change only by extending setup; other records are yours.
