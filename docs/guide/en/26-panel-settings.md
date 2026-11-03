---
id: panel-settings
title: Panel address, license, updates and maintenance
roles: [admin]
topics: [settings, maintenance]
keywords: [settings, port, https, license, update, upgrade, rollback, restart, reboot, sqlite]
---

**Settings** has the tabs **Panel address and HTTPS**, **License**, **CelikPanel updates** and **Remote backups** (see [Backups](guide:backups)). **Maintenance** holds restarts and the system's SQLite databases.

## Address and port {#address}

**Panel certificate** offers **Change address**, **Get certificate** and **Check again**. **Panel port** changes the port with **Change port**: the panel answers on both ports, you confirm from the new address, and without confirmation it returns to the old port after 10 minutes.

## License {#license}

Enter the **License key** and press **Activate license**. Without a valid license the panel shows a license screen instead of its pages; sites, mail, databases and scheduled tasks keep running. If the server cannot reach celikpanel.net, management stays open for 7 days.

## Updating the panel {#update}

Only you, the owner, update the panel, from **CelikPanel updates**, signed in as the primary administrator.

1. Press **Check for updates**. The automatic check only looks; nothing installs by itself.
2. Read **Before you update**: the panel restarts once; websites, mail, DNS and databases keep running.
3. Start the update. The page shows each step (download, signature, read-only check, install, verify) and reconnects by itself.

## If an update does not complete {#rollback}

| You see | What to do |
|---|---|
| **Update stopped before changing the installed version** | Nothing changed. Fix what the message names; trying again is safe. |
| **Update not completed; previous version restored** | The server is back on the previous version. Wait for a corrected release. |
| The result is not known yet | The page keeps reading the server's record. Start nothing meanwhile. |

A rollback restores the previous panel version and the panel database as it was just before the update. Anything done in the panel after the update is lost; check it again. Sites, mail, databases and DNS are not part of it.

## Maintenance {#maintenance}

**Restarts and reboot** shows which services still run replaced code, with **Restart…** for each, and whether a reboot is due. Nothing restarts by itself; you reboot the server yourself.

**System SQLite databases** offers **Check integrity**, **Optimize** and **Download snapshot**. A snapshot holds account data: keep it like a password.
