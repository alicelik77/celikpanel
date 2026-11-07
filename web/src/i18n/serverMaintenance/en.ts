// Route-scoped copy of Server → Maintenance (alpha.128, G29 and G31), like
// ../monitoringAlerts: the server's clock (mclock.), its busiest processes
// (mproc.), restarting the server (mreboot.) and the certificates overview
// with certificate requests (mcert.). It ships with the page's own chunk, so
// the critical boot path does not grow; ../serverMaintenance.ts looks keys
// up in the I18nProvider's locale.
//
// Sunucu → Bakım sayfasının kendi metni: sayfanın paketiyle gelir.

export const enServerMaintenance = {
    'mclock.title': 'Server time',
    'mclock.desc': 'The time this server keeps, its time zone and whether a time service keeps it right. Certificates, mail and scheduled jobs depend on it.',
    'mclock.checking': 'Reading the server clock…',
    'mclock.failed': 'The server clock could not be read, so nothing is shown. Nothing was changed.',
    'mclock.again': 'Read again',
    'mclock.now': 'Server time: {time}',
    'mclock.zone': 'Time zone: {zone} (UTC{offset})',
    'mclock.zoneUnknown': 'Time zone: not named by the system (UTC{offset})',
    'mclock.booted': 'Running since {time}',
    'mclock.synced': 'The clock is synchronised by a time service.',
    'mclock.notSynced': 'No time service is keeping this clock right, so it can drift. Certificates can then fail ("not valid yet") and mail can be refused. You, the administrator: turn time synchronisation on from a terminal with `sudo timedatectl set-ntp true`; `timedatectl` then shows "System clock synchronized: yes" within a few minutes.',
    'mclock.syncUnknown': 'Whether a time service keeps this clock right could not be read. Check it from a terminal with `timedatectl`.',
    'mclock.zoneHint': 'The time zone is changed from a terminal with `sudo timedatectl set-timezone <Region/City>`; nothing that runs on the server depends on the panel for it.',

    'mproc.title': 'Busiest processes',
    'mproc.desc': 'The {n} processes that used the most processor time over one second, and the {n} that use the most memory, read now. Only the program name and its user are shown, never its command line.',
    'mproc.checking': 'Measuring the processes for one second…',
    'mproc.failed': 'The processes could not be read, so none are shown. Nothing was changed.',
    'mproc.again': 'Measure again',
    'mproc.byCPU': 'By processor',
    'mproc.byMemory': 'By memory',
    'mproc.cpu': '{value}% CPU',
    'mproc.total': '{n} processes run on this server ({cpus} processors).',
    'mproc.hidden': 'This server hides other users\' processes from the panel (the hidepid option of /proc), so a list here would show only the panel\'s own. You, the administrator: see them from a terminal with `sudo top` or `ps aux --sort=-%cpu | head -20`.',
    'mproc.empty': 'No process used the processor in that second.',
    'mproc.colName': 'Program',
    'mproc.colUser': 'User',
    'mproc.colValue': 'Use',
    'mproc.noKill': 'To stop a process, restart its service on the Services page, or use `sudo kill <PID>` from a terminal.',

    'mreboot.title': 'Restart the server',
    'mreboot.desc': 'Restarts the operating system, for example after a kernel update. Every site, mailbox, database and this panel stop for a minute or two and start again by themselves.',
    'mreboot.button': 'Restart the server…',
    'mreboot.confirmTitle': 'Restart this server now?',
    'mreboot.confirmMessage': 'Every website, mail service, database and this panel stop for a minute or two, then start again by themselves.\nWork not saved in other sessions or other browser tabs is lost.\nThe restart is refused while an update, a backup or another change is running.',
    'mreboot.confirm': 'Restart now',
    'mreboot.progress': 'Restarting the server',
    'mreboot.step.check': 'Checking that no update, backup or other change is running',
    'mreboot.step.record': 'Recording the restart in the action log',
    'mreboot.step.schedule': 'Asking the server to restart',
    'mreboot.step.down': 'Waiting for the server to stop',
    'mreboot.step.up': 'Waiting for the server to start again',
    'mreboot.step.verify': 'Checking that the server started again',
    'mreboot.downNote': 'The server stops its services now. Nobody needs to act.',
    'mreboot.upNote': 'Back in about {seconds} s. Nobody needs to act; this window waits.',
    'mreboot.upLate': 'Taking longer than usual (a disk check or a slow start can do that). Nobody needs to act yet; this window keeps waiting.',
    'mreboot.done': 'The server restarted and the panel answers again (running since {time}).',
    'mreboot.doneUnverified': 'The panel answers again after stopping, so the server restarted. Its start time could not be read; sign in again if you are asked to.',
    'mreboot.refused': 'The server was not restarted.',
    'mreboot.notRestarted': 'The server did not restart: the panel kept answering and its start time did not change. Nothing else was changed. Try again; if it happens again, restart from a terminal with `sudo systemctl reboot` and check `sudo journalctl -b -1 -n 50` afterwards.',
    'mreboot.noReturn': 'The server has not answered for {minutes} minutes, so whether it started again is not known. You, the owner: open your provider\'s console for this server and look at what the screen shows (a disk check, a boot prompt or a network problem can hold it). This page works again once the server answers.',
    'mreboot.lost': 'The answer to the restart request was lost, so whether the server is restarting is not known. Nobody needs to act yet: wait two minutes and reload this page; the restarts card and the server time say when it last started.',
} as const;

export type ServerMaintenanceKey = keyof typeof enServerMaintenance;
