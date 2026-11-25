// Route-scoped copy of the site limits lane (alpha.149, the alpha.148 lab):
// the move's reasons in the page's own language (D7), a put-back program that
// is not running (D6), an event with the limit that held then (CHECK 2), a
// reached process limit seen by the slice's peak (D3), the reseller's bound
// (CHECK 2), the plan preview's site lines and singular forms, and the Problems
// now lines' reasons (O3). Loaded with the screens that use them (the site's
// card, Problems now, Monitoring, the plan and account editors), never on the
// boot path; ../siteLimitsLane.ts resolves them before the screen catalogue.
//
// Sınır şeridinin ekranla yüklenen metni: taşımanın nedenleri sayfanın
// dilinde, çalışmayan geri konmuş program, olayın o zamanki sınırı.

export const enSiteLimitsLane = {
    'slim.event.memory': 'The programs used all their memory ({time}); the kernel stopped a program {count} times in all.',
    'slim.event.tasks': 'The programs reached their process limit ({time}); new processes could not start, {count} times in all.',
    'slim.event.cpu': 'The programs were slowed down at their processor limit ({time}); nothing was stopped.',
    'slim.event.tasks_peak': 'The programs reached their process limit (seen at the reading of {time}); new processes could not start. This server’s kernel does not count how often.',
    'slim.event.limitThen': 'The limit then: {limit}.',
    'slim.place.restarting': 'not running: it stops and systemd keeps starting it again; its log says why (sudo journalctl -u {name})',
    'slim.who.bound': 'the server’s default, which also bounds the reseller (raised by giving the reseller a limit)',
    'slim.who.boundReseller': 'the server’s default; the server’s administrator can raise it for you',
    'slim.kept.unverified': 'started again; whether it runs was not known yet when the agent started',
    'slim.kept.not_answering': 'runs where it ran but does not answer on its port again',
    'slim.kept.notRunning': '{unit} is not running now: it was put back where it ran, with its earlier file, but did not start again there. Read its own log (sudo journalctl -u {name}), then press Try again or start it on its page. Nothing else was changed.',
    'slim.move.failedUnit': '{unit} did not run inside the limits: {reason}. Its log says why: sudo journalctl -u {name}.',
    'slim.move.back': 'Put back where they ran, running again: {list}.',
    'slim.move.backNot': '{unit}: put back with its earlier file, but {reason}. The card says what to do.',
    'slim.reason.not_running': 'it was not running {seconds} seconds after its restart',
    'slim.reason.restarting': 'it kept stopping, and systemd kept starting it again, for {seconds} seconds after its restart',
    'slim.reason.still_starting': 'it was still starting {seconds} seconds after its restart',
    'slim.reason.not_in_slice': 'it did not run in its place {seconds} seconds after its restart',
    'slim.reason.not_listening': 'it did not answer again on port {ports} within {seconds} seconds',
    'slim.reason.restart_failed': 'the system refused to restart it',
    'slim.reason.write_failed': 'its file could not be written',
    'slim.reason.record_failed': 'the move’s record could not be written',
    'slim.reason.unread': 'where it runs could not be read',
    'slim.reason.other': 'for a reason this page has no words for; the agent’s journal names it (sudo journalctl -u celikpanel-agent)',
    'slim.why.another_move': 'Another move into a site’s limits is running; nothing was changed. Try again when it has finished.',
    'slim.why.site_busy': 'The site is being changed right now (for example deleted); nothing was moved. Read its page again in a minute.',
    'slim.why.kept_earlier': 'An earlier move could not be put back completely, so nothing new was moved. That site’s card names each program left and the next step.',
    'slim.why.unsupported': 'This server cannot hold a site’s limits; nothing was moved.',
    'slim.why.other': 'The move did not run; nothing was moved. The agent’s journal says why (sudo journalctl -u celikpanel-agent).',
    'limits.preview.sitesStopped.one': 'The programs of 1 site use more now than a lowered limit. On saving, a program over the memory limit is stopped at once; at the process limit no new process starts and running ones go on:',
    'limits.preview.part.memory': 'memory {used} in use, new limit {limit}',
    'limits.preview.part.tasks': '{used} processes, new limit {limit}',
    'rights.plan.narrowing.one': '1 account follows this plan. What is taken away keeps running where it is used, but cannot be chosen again. Save?',
    'rights.plan.narrowingReseller.one': '1 reseller follows this plan, with their plans and customers. What is taken away keeps running where it is used, but cannot be chosen again. Save?',
    'pnow.site_move_kept.body_stopped': 'Not running now: {other}. It was put back where it ran, with its earlier file, but did not start again. The server’s administrator reads the program’s own log (sudo journalctl -u with its name), then presses Try again on the site’s card or starts it on its page. Nothing else was changed; no site is moved until this is settled.',
    'pnow.site_limits_unapplied.title_one': 'New site limits have not reached 1 site yet',
    'pnow.site_limits_unapplied.body_one': 'After a change of limits ({domain}), 1 site still has its earlier limits ({other}). CelikPanel tries again every hour; this line goes away when every site has the new ones.',
    'pnow.site_limits_unapplied.title_unchecked': 'Not known whether new site limits reached every site',
    'pnow.site_limits_unapplied.body_unchecked': 'After a change of limits ({domain}), what the running programs hold could not be read for sites: {value}. So it is not known whether they have the new limits. CelikPanel tries again every hour; the agent’s journal says why (sudo journalctl -u celikpanel-agent).',
    'slim.applyReason.kernel_differs': 'the kernel still holds the earlier values',
    'slim.applyReason.write_failed': 'their limits file could not be written',
    'slim.applyReason.refused': 'their limits file was refused',
    'slim.applyReason.out_of_range': 'the limits are out of range',
    'slim.applyReason.agent_unreachable': 'the panel’s agent did not answer',
    'slim.applyReason.agent_refused': 'the panel’s agent did not take the change',
    'slim.applyReason.other': 'the agent’s journal says why: sudo journalctl -u celikpanel-agent',
} as const;

export type SiteLimitsLaneKey = keyof typeof enSiteLimitsLane;
