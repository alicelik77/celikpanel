---
id: monitoring-audit
title: Monitoring, the audit log and notices
roles: [admin]
topics: [monitoring, audit]
keywords: [cpu, memory, disk, load, graph, audit, log, history, who did what, notice, alert, uptime, services, email]
---

## Monitoring {#monitoring}

**Monitoring** starts with **Problems now** and **Alerts**, then draws **Processor**, **Memory**, **Disk (/)** and **Load (1m)**: each point is the average of 30 seconds, kept for 30 days; choose **1 hour** to **30 days**. The disk card adds disk read and write speed, the load card network in and out. When the processor or memory reaches 90%, the five busiest processes of that moment appear under the chart.

**Services** lists each installed service: running since when, restarts in the last hour, processor and memory. **Websites** asks each site for its home page from this server every 5 minutes and shows whether it answers and how many days its certificate has left.

A problem that lasts longer than the delay in **Alert settings** (10 minutes by default) becomes an alert. Alerts are emailed only to administrators whose address is confirmed on **My account**, at most four emails an hour. While the panel or the server is stopped there are no alerts: also use an outside uptime checker for important sites.

## Audit log {#audit}

**Audit log** lists who did what, newest first: **Time**, **User**, **Action**, **Detail** and **IP**. Work done with **Log in as** shows both names. The page shows the latest 200 entries, without filter or search yet; the dashboard shows the newest under **Recent activity**.

If the log looks empty on a server that has been used, reload the page: the list may not have been read.

## Notices {#notices}

| Where | What |
|---|---|
| Dashboard, **Needs attention** | A stopped service, the firewall off or not saved, certificate renewal problems, mail without a spam filter. |
| Dashboard | A new panel version, system updates, license problems. |
| **Users**, **Traffic and disk notices** | Accounts and reseller pools at 80% or 100%. |
| **Monitoring**, **Alerts** | Problems that lasted longer than the alert delay; the Dashboard shows how many are new. |
| Email | Monitoring alerts to confirmed administrators, usage notices, malware findings, security emergency notices to customers. |
