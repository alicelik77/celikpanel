// SCREEN catalogue part "opsteps" (alpha.127): the real steps the progress
// window shows while a change runs - issuing a certificate, restoring or
// creating a backup, changing a site's PHP version, installing an
// application and the WordPress tools - and what it says about a step that
// stopped (opstep.). The owner's firm order of 6 Nov 2026: say what is being
// done, step by step, until it ends.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "opsteps" ekran parçası: bir değişiklik sürerken ilerleme penceresinin
// gösterdiği gerçek adımlar.

export const enOpstepsScreens = {
    'opstep.stoppedHere': 'Stopped at this step. The steps after it did not run.',
    'opstep.stoppedBeforeChange': 'Stopped at this check, before anything was changed.',
    'opstep.stepUnknown': 'It is not known whether this step finished.',
    'opstep.unread': 'The server has not answered about this change for a minute, so how it ended is not known. Nothing was sent again. Reload the page and check the current state before trying again.',
    'opstep.gone': 'The server keeps no record of this change any more, so how it ended is not known. Nothing was sent again. Check the current state on this page.',
    'opstep.interrupted': 'CelikPanel restarted while this change was running, so it is not known whether it finished. It will not run again by itself. Check the current state on this page and make the change again only if it is missing.',
    'opstep.refusedNotKept': 'The change ended with an error whose text is not kept. Nothing was sent again. Check the current state on this page, then try again.',

    'opstep.ssl.title': 'Getting a certificate for {name}',
    'opstep.ssl.check': 'Checking the request',
    'opstep.ssl.dns_check': 'Checking that {name} points to this server',
    'opstep.ssl.prepare_validation': 'Preparing the web server for the authority’s check',
    'opstep.ssl.acme': 'Asking {authority} and proving control of the domain',
    'opstep.ssl.acmeNote': 'This usually takes under a minute: the authority visits the site to check it.',
    'opstep.ssl.verify': 'Checking the new certificate',
    'opstep.ssl.install': 'Installing the certificate',
    'opstep.ssl.reload': 'Reloading the web server',
    'opstep.ssl.mail': 'Moving mail encryption to the new certificate',

    'opstep.restore.title': 'Restoring {name}',
    'opstep.restore.check': 'Checking the disk space limit',
    'opstep.restore.inspect': 'Reading the backup and checking it is safe to restore',
    'opstep.restore.restore': 'Putting the files and databases back',
    'opstep.restore.restoreNote': 'A large site can take several minutes. You do not need to do anything.',
    'opstep.restore.finish': 'Finishing',

    'opstep.backup.title': 'Backing up {name}',
    'opstep.backup.check': 'Checking the disk space limit',
    'opstep.backup.create': 'Creating the backup',
    'opstep.backup.createNote': 'A large site can take several minutes. You do not need to do anything.',
    'opstep.backup.finish': 'Finishing',

    'opstep.php.title': 'Changing {name} to PHP {version}',
    'opstep.php.check': 'Checking that PHP {version} is available',
    'opstep.php.pool': 'Moving the site to PHP {version}',
    'opstep.php.save': 'Saving the change',
    'opstep.php.web_server': 'Pointing the web server at PHP {version} and reloading it',

    'opstep.app.title': 'Installing {name}',
    'opstep.app.check': 'Checking what {name} needs',
    'opstep.app.database': 'Creating the database',
    'opstep.app.files': 'Downloading {name}, checking it and putting the files in place',
    'opstep.app.setup': '{name} is setting itself up',
    'opstep.app.finish': 'Finishing',
    'opstep.app.reloadedNote': 'This install was already running when the page opened; it is followed here.',

    'opstep.wp.start': 'Sending the request',
    'opstep.wp.queued': 'Waiting for another WordPress job on this server to finish',
    'opstep.wp.backing_up': 'Backing up the site first',
    'opstep.wp.updating': 'Updating WordPress',
    'opstep.wp.cloning': 'Copying the site to {target}',
    'opstep.wp.restoring': 'The update did not pass its check: putting the backup back',
    'opstep.wp.finish': 'Checking the result',

    'opstep.common.reload': 'Reloading the web server',
    'opstep.common.mail': 'Updating mail encryption',
    'opstep.common.records': 'Updating CelikPanel’s records',

    'opstep.sslUpload.validate': 'Checking the certificate, its key and chain',
    'opstep.sslUpload.install': 'Saving the certificate on the server',
    'opstep.sslRemove.detach': 'Taking the certificate off the site',
    'opstep.mailCert.title': 'Changing mail encryption of {name}',
    'opstep.mailCert.check': 'Checking the certificate covers mail.{name}',
    'opstep.mailCert.mail_tls': 'Setting mail encryption and reloading the mail servers',
    'opstep.mailCert.tlsa': 'Updating the TLSA records in DNS',

    'opstep.dns.saveTitle': 'Saving a DNS record of {name}',
    'opstep.dns.deleteTitle': 'Deleting a DNS record of {name}',
    'opstep.dns.check': 'Checking the record',
    'opstep.dns.save': 'Saving the record',
    'opstep.dns.publish': 'Publishing the zone to the DNS servers',
    'opstep.pdns.title': 'Repairing PowerDNS',
    'opstep.pdns.check': 'Checking that DNS is free for a repair',
    'opstep.pdns.configure': 'Repairing PowerDNS’s configuration',
    'opstep.pdns.sync': 'Publishing every zone again',

    'opstep.phpIni.title': 'Applying php.ini for PHP {version}',
    'opstep.phpIni.check': 'Checking the settings',
    'opstep.phpIni.apply': 'Writing php.ini and reloading PHP {version}',
    'opstep.hosting.title': 'Changing the hosting type of {name}',
    'opstep.hosting.check': 'Checking the new hosting type',
    'opstep.hosting.switch': 'Switching the site and reloading the web server',

    'opstep.service.check': 'Checking the request',
    'opstep.service.start': 'Starting {name} and checking it runs',
    'opstep.service.stop': 'Stopping {name} and checking it stopped',
    'opstep.service.restart': 'Restarting {name} and checking it runs',
    'opstep.service.refresh': 'Reading the components’ state again',

    'opstep.malware.check': 'Checking the finding',
    'opstep.malware.quarantine': 'Moving the file to quarantine',
    'opstep.malware.restore': 'Putting the file back',
    'opstep.malware.delete': 'Deleting the quarantined file',
    'opstep.malware.save': 'Recording the result',

    'opstep.dbDelete.check': 'Checking the database',
    'opstep.dbDelete.delete': 'Deleting {name} on the database server',
    'opstep.dbUserDelete.check': 'Checking the database user',
    'opstep.dbUserDelete.delete': 'Deleting the user {name} on the database server',
    'opstep.adminRemove.check': 'Checking CelikPanel’s account',
    'opstep.adminRemove.remove': 'Removing CelikPanel’s account from the database server',
    'opstep.userDelete.delete': 'Checking {name} owns nothing and deleting the account',
} as const;

export type OpstepsScreenKey = keyof typeof enOpstepsScreens;
