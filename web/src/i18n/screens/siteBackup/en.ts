// SCREEN catalogue part "siteBackup" (the release after alpha.133,
// docs/SITE-BACKUP-CONTENTS-DESIGN.md): what a backup of a site holds - its
// files, the program's files and data, a service app's data - what it leaves
// out and why, the warning before a large backup, the backups that do not
// hold the program, and what a restore did with the program. Key prefixes
// kept here: bkc., and the refusals err.BACKUP_RESTORE_PROGRAM_*. Every other
// backup. key stays in "sites".
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "siteBackup": bir site yedeğinin neleri içerdiği,
// dışarıda kalanlar ve geri yüklemenin programla ne yaptığı.

export const enSiteBackupScreens = {
    'bkc.title': 'What a backup of this site holds',
    'bkc.checking': 'Reading what a backup of this site holds…',
    'bkc.unknown': 'What a backup of this site holds could not be read. You can still make a backup; the list shows what each backup holds.',
    'bkc.site': 'Site files ({size})',
    'bkc.filesDescAreas': 'The site’s files, with its program and app data',
    'bkc.fullDescAreas': 'The site’s files, program and app data + all {count} linked databases',
    'bkc.area.program': 'Program files and data ({size})',
    'bkc.area.app': '{app} data ({size})',
    'bkc.databases': 'A full backup also holds the databases: {names}',
    'bkc.leftTitle': 'Not in the backup:',
    'bkc.left.cache': 'caches and unfinished build folders, made again by the next deploy ({size})',
    'bkc.left.reinstalled': 'the installed program files of service apps, which the panel installs again ({size})',
    'bkc.left.special': '{count} special files (sockets or pipes) the program makes again when it starts',
    'bkc.left.foreign': '{count} items that do not belong to the site’s account: {paths}',
    'bkc.left.other': '{count} items in place of a program folder: {paths}',
    'bkc.left.more': 'and {count} more',
    'bkc.secrets': 'The program’s secret settings are never written into a backup. A restore on this server keeps the ones the server has; on a new server, enter them again on the Programs tab.',
    'bkc.appsNotRestored': '{apps}: the backup keeps the data, but the panel does not restore it in this release. The backup can be downloaded and opened by hand.',
    'bkc.atLeast': 'Counting stopped at its time limit: the backup is at least this large.',
    'bkc.large': 'This backup is large ({size}): making it can take several minutes, and it takes about this much space on the server.',
    'bkc.space.title': 'The server’s backup disk may not have room for this backup',
    'bkc.space.why': 'Free: {free}. The backup needs up to {size}.',
    'bkc.space.next': 'Free space on the server, or ask the administrator, before you make a files or full backup.',
    'bkc.confirmLarge': 'This backup will be about {size}. Making it can take several minutes. Make it now?',
    'bkc.confirmLargeLabel': 'Make the backup',
    'bkc.row.holds': 'Holds: {list}',
    'bkc.row.noProgram': 'This backup does not hold the program',
    'bkc.row.noProgramOld': 'This backup does not hold the program (older format)',
    'bkc.row.left': 'Not in this backup: {list}',
    'bkc.restore.stopStart': 'If the program is running, it is stopped for the restore and started again after it.',
    'bkc.restore.programLost': 'The program’s current files and data are replaced by the backup’s',
    'bkc.restore.secretsKept': 'The program’s settings and secret values on this server stay as they are',
    'bkc.restore.programKept': 'The program stays as it is: this backup does not hold it',
    'bkc.restore.appsKept': '{apps} stays as it is: the panel does not restore it in this release',
    'bkc.result.started': 'Restored. The program was started again and answers.',
    'bkc.result.notStarted': 'The backup was restored, but the program did not start',
    'bkc.result.leftStopped': 'The backup was restored; the program was left stopped',
    'bkc.result.failedLeftStopped': 'The program was left stopped',
    'bkc.result.failedStarted': 'The program was started again on its earlier files.',
    'bkc.result.unknown': 'Whether the program started again is not known',
    'bkc.result.notSetUp': 'The program’s files were restored, but no program is set up on this site',
    'bkc.why.held': 'The administrator has put programs on a security hold.',
    'bkc.why.suspended': 'This subscription is suspended.',
    'bkc.why.sandbox_reduced': 'This server does not enforce the full sandbox, and the administrator has not accepted that.',
    'bkc.why.files_unknown': 'Putting the earlier files back did not finish, so the program’s files are not in a known state. The safety copy is kept.',
    'bkc.why.restore_unknown': 'The server did not answer during the restore, so whether it finished is not known.',
    'bkc.why.gate_unread': 'Whether the program may start could not be read.',
    'bkc.why.restart_loop': 'It stopped again and again right after starting.',
    'bkc.why.not_listening': 'It runs but did not answer on its port in time.',
    'bkc.why.start_failed': 'It stopped while starting.',
    'bkc.why.oom': 'It used more memory than its limit.',
    'bkc.why.port_taken': 'Another program already uses its port.',
    'bkc.why.listens_elsewhere': 'It listened on an address other than this server’s own, so it was stopped.',
    'bkc.why.other': 'The server gave another reason; the Programs tab shows it.',
    'bkc.next.programs': 'Open the Programs tab: its log shows why. Start the program there when it is fixed.',
    'bkc.next.startLater': 'Start the program on the Programs tab once this is resolved.',
    'bkc.next.check': 'Open the Programs tab to see whether it runs, and start it there if it does not.',
    'bkc.next.setUp': 'Set the program up on the Programs tab and deploy it.',
    'err.BACKUP_RESTORE_PROGRAM_BUSY': 'The site’s program is being deployed or removed right now, so the restore was not started and nothing changed. Wait until that ends on the Programs tab, then restore again.',
    'err.BACKUP_RESTORE_PROGRAM_UNKNOWN': 'The server did not answer about the site’s program, so the restore was not started or its result is not known. Check the site and the Programs tab; start the program there if it is stopped.',
    'err.BACKUP_RESTORE_PROGRAM_STOP_FAILED': 'The site’s program could not be stopped, so the restore was not started and nothing changed. The server’s reason is below; check the program on the Programs tab.',
} as const;

export type SiteBackupScreenKey = keyof typeof enSiteBackupScreens;
