---
id: ai-apps
title: AI and automation apps
roles: [admin, reseller, customer]
topics: [domain.site.ai#ai-apps]
keywords: [ai, n8n, open webui, automation, workflow, chat, openai, api key, security hold]
---

**AI & automation**, in a domain's **Site** section, runs an app such as n8n (workflow automation) or Open WebUI (a chat screen for AI models) on a domain. The app runs as the site's own account in a closed sandbox, listens only inside the server, and opens at the domain's address over HTTPS.

## Before you install {#ai-apps}

The page says what is still needed, and who provides it:

- **A website and a certificate.** Apps open only over HTTPS. If the domain has no certificate yet, get one first in the **Security** section.
- **A separate subdomain**, such as `n8n.example.com`, is best: the domain shows the app instead of its website. The website's files stay and come back when the app is removed.
- **The offer.** Nothing can be installed until the administrator offers the app, and your plan must include it. Otherwise the row says whom to ask.
- **The server.** n8n needs Node.js 24, which the administrator installs. Open WebUI runs only on some systems; the row says so when it cannot run here.

## Installing {#installing}

1. Press **Install** beside the app.
2. Enter the **App administrator’s e-mail** and **App administrator’s password** (12 to 128 characters). You sign in to the app with these, so nobody else can claim it first. CelikPanel does not keep the password.
3. For Open WebUI you may add **AI provider keys (optional)**. They are kept only in the app's own settings file and are never shown again.
4. Tick that you have read what the app can do and what remains a risk, and confirm.

The install runs on the server and may take several minutes; Open WebUI is several GB. You can close the page; the page shows each step when you come back. If a step fails, the message says why and who acts; the install log shows details.

## Using the app {#running}

**Open the app** opens it in a new tab. The row shows whether it is **Running**, **Stopped** or **Running, but not answering yet**, and how much memory it uses. You can **Start**, **Stop**, **Restart** it and read its **Log**. **Change keys** replaces the provider keys or sets a new administrator password.

Memory, processor and process limits are set by the administrator for every install. When a newer version comes with a panel release, **Update to …** installs it beside the running one and switches over only when it is ready.

## Security hold {#hold}

The administrator can put an app on a security hold, for example when a security problem is published. All installs of that app stop and cannot be started or installed until the hold is lifted. Your data stays. Lifting the hold starts nothing: start the app again yourself.

## Why n8n is not available through a reseller {#n8n-resellers}

n8n's licence restricts offering it to others as a hosted service. It is therefore closed to resellers and their customers, and only the administrator's own customers can use it. The row says so; there is nothing for you to do. Open WebUI is not affected.

## Removing the app {#removing}

**Remove the app** removes the service and its program files; the domain shows its website again. The app's data (workflows, chats, its database) stays in its folder unless you tick **Also delete the app’s data** and type the domain.

The app does not depend on the panel. It is an ordinary system service on the server; the page names it, and the server's owner can stop it with `sudo systemctl stop` followed by that name.
