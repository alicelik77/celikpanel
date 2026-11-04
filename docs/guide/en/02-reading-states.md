---
id: reading-states
title: Reading a state and getting help
roles: [admin, reseller, customer]
topics: [states]
keywords: [error, problem, waiting, retry, check again, unknown, stuck, failed, support]
---

Every screen in CelikPanel tells you the same four things when something needs attention: the reason, who has to act, the next step, and how the work goes on afterwards. This chapter shows how to read them, so that you know whether to wait, act, or ask someone.

## Checking, could not check, known {#three-states}

Whatever a screen reads from the server is shown in one of three ways, and they never look alike.

| You see | It means | What to do |
|---|---|---|
| A small spinner and a sentence ending in "…" | The panel is asking the server right now. Nothing has gone wrong. | Wait a moment. |
| A yellow notice with **Retry** | The panel could not read the answer. This does not mean the thing is missing or broken, and nothing was changed. | Press **Retry**. If it keeps happening, see [When the panel itself is not answering](#panel-not-answering). |
| The answer itself | The server answered. Only now does the screen say "none", "off", "empty" or show a number. | Read it, and act if you want to. |

## What a problem tells you {#problem}

A message that stops you stands at the top of what it blocks. Read it in this order:

1. **What is wrong**, in one plain sentence, with the domain, mailbox or component named.
2. **Who has to act.** It may be you, the server administrator, your provider, a DNS provider or registrar, or the owner of another server.
3. **The next step**, as concrete as the panel can make it. Where the panel can do the step itself, there is a button beside the sentence.
4. **How it goes on**: the panel checks again by itself, you press a button such as **Check again**, or nothing more is needed.

A button you cannot press has its reason written next to it. The internal code of a problem stays under **Technical details**; you only need it when you ask someone for help.

## Failed, waiting, or not known {#failed-waiting-unknown}

The panel keeps three situations apart, because each asks something different of you.

- **It failed.** The server tried and it did not work. The message says what was tried and what is in place now, usually the previous state. Fix the cause the message names, then try again.
- **Something is needed first.** Nothing was tried, because something has to be ready before it: a plan that includes the feature, a component on the server, a DNS record at your registrar. The message names that thing and who provides it.
- **The result is not known.** The change was sent, but its answer did not arrive, for example because the connection dropped. The panel does not send it a second time. It reads the current state again and then tells you whether the change was made. Use **Check again** for that; it only reads.

:::warning
When a result is not known, do not repeat the change from another tab or browser. Wait for **Check again** to show what happened first.
:::

## How work goes on {#resume}

Long jobs, such as installing a component, taking a backup or issuing a certificate, run on the server and not in your browser. If you close the page or lose the connection, the job continues. When you come back, the page shows where it is. A page that is waiting moves or says that you do not need to do anything; you never need to reload the page to see a result.

## Asking for help {#asking}

If you cannot solve a problem yourself, write to whoever the message names: your provider for a customer or user account, the server administrator for anything on the server. Send them:

- the page and the domain, mailbox or component it was about;
- the sentence the message says, and the code under **Technical details**;
- the time it happened.

Never send a password or a private key. CelikPanel never shows them back to you, and nobody needs them to help.

## When the panel itself is not answering {#panel-not-answering}

If the panel cannot be reached or says it is starting, your websites, mail, DNS, databases and scheduled jobs keep running. They are ordinary services of the server and do not depend on the panel. Certificates of new websites are renewed by the server itself. Three kinds still need CelikPanel to renew them: the panel's own certificate, the mail server's certificate and older website certificates that have not been issued again. They do not expire at once, so a short outage is harmless. Try again in a few minutes. If the panel stays unreachable, tell the server administrator.
