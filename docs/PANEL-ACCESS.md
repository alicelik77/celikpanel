# Panel sessions and administrator access by address

Status: alpha.128 (gap item G4). Turkish: [PANEL-ACCESS.tr.md](PANEL-ACCESS.tr.md).

## What it does

- **Signed-in sessions** (My account): every browser where the account is signed in, with the
  address and browser it last used, "End" for one session and "End all other sessions". A
  password change already ends every session of the account.
- **Every account's sessions** (Security, administrator): who is signed in now, with "End".
- **Idle timeout** (Security → Panel access): a session that was not used for 15, 30, 60
  (default), 120, 240 or 480 minutes ends at its next request. Typing, clicking, touching the
  page and every change count as use; pages that refresh by themselves do not.
- **Administrator addresses** (Security → Panel access): when the list is not empty,
  administrator sign-ins and administrator sessions (including "Log in as" sessions an
  administrator opened) are accepted only from the listed addresses and networks.
  Customers and resellers are never affected. The list is the panel's own: SSH, mail,
  websites and the firewall do not change.

## Lock-out safety

1. The address that saves a non-empty list must be in it; otherwise the panel refuses to save
   and says why. The screen offers "Add my address".
2. The server itself (127.0.0.1 and ::1) is always allowed, so an SSH tunnel always works:

   ```
   ssh -L 1923:127.0.0.1:1923 root@your-server
   ```

   then open `https://localhost:1923` (use your panel port).
3. **Local recovery command.** On the server, as root:

   ```
   sudo touch /etc/celikpanel/panel-access-open
   ```

   Within a few seconds administrators may sign in from any address. Correct the list under
   Security → Panel access, then remove the file:

   ```
   sudo rm /etc/celikpanel/panel-access-open
   ```

   While the file exists, the Panel access card says so. Nothing else reads this file.

## Where it is stored

`panel_settings` rows `panel.sessions.idle_minutes` and `panel.access.allowlist` (a JSON list).
Sessions gain `last_seen_at`, `client_ip` and `user_agent` (migration 072); `last_seen_at` is
written at most once a minute per session. The address is the connection's own address
(`RemoteAddr`); the panel is reached directly, not through a proxy.

## Audit

`auth.session.end`, `auth.session.end_others`, `panel.access.settings`, and
`auth.login.address_refused` for an administrator sign-in refused by the list.
