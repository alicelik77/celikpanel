---
id: dns
title: DNS for your domain
roles: [admin, reseller, customer]
topics: [domain.dns#records]
keywords: [dns, zone, record, a record, mx, txt, cname, nameserver, registrar, dnssec, ds]
---

DNS tells the internet where your website and mail are. The **DNS records** part of the domain's **Domain & DNS** section shows its zone: the list of records that answer for the name.

## Where your DNS is served {#where}

It tells you which of three cases applies:

- **Served by this server.** The records are listed and you can change them. Changes are published to the server's DNS service, and to its second server if there is one.
- **DNS is managed at your provider.** The domain uses another company's nameservers. The page lists the values to add there. This list is what is needed, not proof that it is published; the **Connecting this domain** card, on **Overview** and at the top of **Domain & DNS**, checks the public records.
- **Managed through an authorized CelikPanel DNS connection.** Another CelikPanel server holds the zone and publishes your changes.

## Adding and removing records {#records}

1. Press **Add record**.
2. Choose the **Type**: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS` or `SRV`.
3. Type the **Name** (`@` for the domain itself, `www` for www.example.com), the **Value** and the **TTL**. `MX` and `SRV` also take a **Priority**.
4. Press **Save record**.

To change a record, delete it with the bin icon and add it again. **Refresh records** reads the zone again.

Some records cannot be changed here. A record marked as managed in **Server → DNS and peer servers** belongs to the server's own setup. If your plan does not include DNS editing, the records are shown but cannot be changed; your provider can add it.

:::warning
A wrong `A`, `MX` or `NS` record can take your website or mail offline for as long as the TTL. Note the old value before you delete a record.
:::

## Pointing the domain here {#pointing}

The domain reaches this server only after you set it at the company you bought it from (the registrar). The **Connecting this domain** card, on **Overview** and at the top of **Domain & DNS**, gives the exact values: the nameservers for Route A, or the address records for Route B. See [The overview](guide:domains-websites#overview).

## What the owner of the DNS servers does {#dns-owner}

The administrator of the server runs the DNS software (BIND or PowerDNS), chooses the nameserver names and connects a second server, under **Server → DNS and peer servers**. The second server copies your zone by the standard zone transfer; it needs no CelikPanel action from you.

| The page says | Who acts | What to do |
|---|---|---|
| DNS zone is not active | You or the administrator | Press **Enable DNS zone**. |
| The records were not published | The administrator | Fix the DNS server problem named beside it, then press **Republish DNS**. |
| The shared nameserver setup is not ready yet | The administrator | Finish the nameservers; until then, use Route B. |

Nothing has to be reloaded: once the records are published, the page shows them.

## Signing the zone (DNSSEC) {#dnssec}

**Sign the zone** adds DNSSEC signatures. The page then shows the DS values to enter at your registrar. Enter them last, only after both nameservers serve the signed zone: a DS record entered too early or wrongly makes the domain unreachable for visitors whose resolvers check DNSSEC. If the option is missing, your plan does not include it.
