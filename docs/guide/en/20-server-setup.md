---
id: server-setup
title: First setup of the server
roles: [admin]
topics: [server-setup]
keywords: [wizard, setup, first server, join code, second server, nameserver, roles]
---

The setup wizard prepares a fresh server for websites, mail and DNS. It shows its plan first and changes nothing until you start it. Existing data is kept.

## Wizard or by hand {#choice}

On first sign-in the panel asks how to prepare the server. **Set up with the wizard** is the usual way. If you choose to configure it yourself, you can open the wizard later from **Server setup**.

## The steps {#steps}

1. **This server:** **The first server**, or **Join an existing server** for a second one. The first server asks for your main domain.
2. **Number of servers:** **One server** or **Two servers**. With two, the second keeps a copy of the DNS.
3. **What it will do:** tick **Websites**, **Email**, **DNS (domain list)**. Each shows what it installs. A firewall that keeps SSH and panel access, and certificate renewal, are always included.
4. **Names:** the panel address and the mail server address.
5. **Where the DNS list lives:** see [DNS choices](#dns).
6. **Summary:** what will be set up and what will not be touched. Tick the confirmation and press **Start reviewed setup**.

Other steps appear only when they apply, for example existing records to take over. Anything that blocks the start is listed first, with a button to fix it or check again.

## DNS choices {#dns}

| Choice | What happens |
|---|---|
| **On my own servers** | This server runs BIND or PowerDNS. With two servers, one is the main DNS server and the other keeps a copy. |
| **At an external DNS provider** | No DNS software is installed. The panel shows the records to add at your provider; it does not write to the provider. |
| **On another CelikPanel** | The domain list lives on another panel, connected with a one-time connection code. |

## While it runs {#progress}

Each step shows **Waiting**, **In progress**, **Completed**, **Failed** or **Waiting for you**. The panel restarts once when it secures its address. You can close the page; setup goes on.

**What to enter at your registrar** lists the nameservers and their IP records (glue). Only you can enter them at your registrar; the card turns correct by itself once the internet sees them.

## Joining a second server {#join}

1. On the first server, press **Create a join code**. The code is shown once and expires.
2. On the second server, choose **Join an existing server**, enter the first server's address and the code, and press **Join**.

## When it is done {#done}

**Your server is ready** lists what is left for you, each with a button. Then add your first website. **Extend setup** adds roles later; it never removes anything. More on DNS: [DNS and peer servers](guide:dns-servers).
