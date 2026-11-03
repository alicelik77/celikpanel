---
id: recovery
title: Recovery basics for the administrator
roles: [admin]
topics: []
keywords: [recovery, unknown, stuck, not answering, check again, journalctl, rollback, help]
---

Use the panel's own screens first: they say the reason, who acts, the next step and how work resumes. Go to the terminal only when a screen tells you to, or when the panel cannot be reached at all.

## When a result is not known {#unknown}

"Not known" is not "failed": the answer was lost, and the panel does not send the change again.

1. Press **Check again**. It only reads.
2. It shows whether the change was made, was not made, or is still running.
3. Only if it was not made, start it again.

:::warning
While a result is unknown, do not repeat the change from another tab, the terminal or a script. Two changes at once can leave a state neither meant.
:::

## When the panel does not answer {#not-answering}

A notice that the panel did not answer usually clears by itself, for example while an update restarts it. If it stays, press **Check panel access**. Hosted services keep running (see [Running the server without CelikPanel](guide:without-panel)); `sudo systemctl status celikpanel-panel celikpanel-agent` shows whether the panel's own services run.

After an update that did not complete, **Update and recovery status** shows what happened. If automatic recovery pauses, that screen gives the exact read-only command for its log; run that one. See [If an update does not complete](guide:panel-settings#rollback).

## Before you change things by hand {#by-hand}

- Read **Technical details** first; its code names the exact problem.
- Change one thing at a time, then let the page read the state again.
- Note what you changed, for anyone who helps you.
- Never hand out a password, private key or license key.
