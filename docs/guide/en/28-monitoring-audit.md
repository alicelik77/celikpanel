---
id: monitoring-audit
title: Monitoring, the audit log and notices
roles: [admin]
topics: [monitoring, audit]
keywords: [cpu, memory, disk, load, graph, audit, log, history, who did what, notice, alert]
---

## Monitoring {#monitoring}

**Monitoring** draws **CPU**, **Memory**, **Disk (/)** and **Load (1m)**, sampled every 30 seconds and kept for 48 hours; choose **1h** to **48h**. A fresh server needs a few minutes for its first samples. Service states are on the dashboard under **System services**. The panel sends no health alerts by email.

## Audit log {#audit}

**Audit log** lists who did what, newest first: **Time**, **User**, **Action**, **Detail** and **IP**. Work done with **Log in as** shows both names. The page shows the latest 200 entries, without filter or search yet; the dashboard shows the newest under **Recent activity**.

If the log looks empty on a server that has been used, reload the page: the list may not have been read.

## Notices {#notices}

| Where | What |
|---|---|
| Dashboard, **Needs attention** | A stopped service, the firewall off or not saved, certificate renewal problems, mail without a spam filter. |
| Dashboard | A new panel version, system updates, license problems. |
| **Users**, **Traffic and disk notices** | Accounts and reseller pools at 80% or 100%. |
| Email | Usage notices, malware findings, security emergency notices to customers. |
