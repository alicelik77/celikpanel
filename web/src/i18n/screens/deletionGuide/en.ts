// SCREEN catalogue part "deletionGuide" (alpha.133): a domain deletion that
// waits for its DNS step, said in the order of D-024 - why, who acts, the
// next step, how it continues - for the reasons CelikPanel's own checks
// handle (the backup DNS server catching up, a check that ran out of time,
// this server's check record) and the one that needs the administrator.
// The banner on Domains, the deletion window and the server's notification
// (cmd/panel dns_pending_publication.go) use these same sentences. Key
// prefix: domains.deletionGuide.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "deletionGuide": DNS adımını bekleyen silme; neden,
// kim yapar, sonraki adım, nasıl sürer.

export const enDeletionGuideScreens = {
    // Why, one sentence each.
    'domains.deletionGuide.journalUnknown.why': 'The domain is removed from DNS on this server; that the backup DNS server removed it too is not confirmed yet, because this server could not complete its last check.',
    'domains.deletionGuide.journalUnresolved.why': 'The domain is removed from DNS on this server, but the record this server keeps to confirm that the backup DNS server removed it too is in a state CelikPanel cannot sort out by itself.',
    'domains.deletionGuide.proofTimeout.why': 'The domain is removed from DNS on this server; that the backup DNS server removed it too is not confirmed yet, because the check did not finish in time.',
    'domains.deletionGuide.catchingUp.why': 'The domain is removed from DNS on this server; the backup DNS server has not caught up with it yet.',
    // Who acts.
    'domains.deletionGuide.who.nobody': 'You do not need to do anything.',
    'domains.deletionGuide.who.youAdministrator': 'You, as this server’s administrator.',
    'domains.deletionGuide.who.administrator': 'This server’s administrator.',
    // The next step of a person.
    'domains.deletionGuide.journalUnresolved.next': 'Send the two command outputs under Details to CelikPanel support, and do not delete those files yourself.',
    'domains.deletionGuide.journalUnresolved.nextWindow': 'Open Details in the waiting notice at the top of Domains and send the two command outputs it shows to CelikPanel support; do not delete those files yourself.',
    'domains.deletionGuide.journalUnresolved.nextOther': 'Tell this server’s administrator (your hosting provider); nothing is needed from you.',
    'domains.deletionGuide.stuck.next': 'Follow the step in the notification CelikPanel sent about this deletion.',
    'domains.deletionGuide.stuck.nextOther': 'Tell this server’s administrator (your hosting provider) if this stays.',
    'domains.deletionGuide.stopped.next': 'Read the CelikPanel Agent’s lines with the command under Details and fix what they name.',
    'domains.deletionGuide.stopped.nextWindow': 'Open Details in the waiting notice at the top of Domains, read the CelikPanel Agent’s lines with the command it shows and fix what they name.',
    // How it continues.
    'domains.deletionGuide.then.at': 'CelikPanel checks again by itself at about {time}; once that is confirmed, the deletion finishes by itself.',
    'domains.deletionGuide.then.now': 'CelikPanel is checking it again right now; once that is confirmed, the deletion finishes by itself.',
    'domains.deletionGuide.then.soon': 'CelikPanel checks again by itself within a few minutes; once that is confirmed, the deletion finishes by itself.',
    'domains.deletionGuide.then.stuck': 'CelikPanel keeps checking every 5 minutes; once the cause is gone, the deletion finishes by itself.',
    'domains.deletionGuide.then.person': 'CelikPanel no longer checks this every few minutes. Once the record is sorted out, More → Delete… on this domain’s row continues this same deletion.',
    'domains.deletionGuide.then.personOther': 'CelikPanel no longer checks this every few minutes; once this server’s administrator has sorted it out, the deletion continues from where it stopped.',
    'domains.deletionGuide.then.stopped': 'CelikPanel’s automatic checks of this step have ended. Once the cause is gone, More → Delete… on this domain’s row continues this same deletion.',
    'domains.deletionGuide.then.stoppedOther': 'CelikPanel’s automatic checks of this step have ended; once this server’s administrator has dealt with the cause, the deletion continues.',
    'domains.deletionGuide.then.stoppedSupport': 'CelikPanel’s automatic checks of this step have ended, and this page offers no retry while the backup catches up; once the cause is fixed, CelikPanel support helps continue the deletion.',
    // Behind the closed "Details".
    'domains.deletionGuide.stepNote.person': 'Waits for this server’s administrator.',
    'domains.deletionGuide.details': 'Details',
    'domains.deletionGuide.journalUnknown.details': 'To confirm the backup DNS server, this server keeps a check record that each check uses once. Its last check could not complete that record; the next check starts a new one and also finishes a change of the record that an earlier check left half done. If the check stays stuck, the server administrator reads the CelikPanel Agent’s lines about it with this command:',
    'domains.deletionGuide.journalUnresolved.details': 'To confirm the backup DNS server, this server keeps a check record (files whose names contain dns-peer in /var/lib/celikpanel-agent-private). CelikPanel finishes by itself a change of that record that an earlier check left half done; what it found here is not such a change, so it keeps the files as they are for review. The server administrator runs these two commands on this server and sends their output:',
    'domains.deletionGuide.proofTimeout.details': 'If the check stays stuck for about a quarter of an hour, the server administrator gets a notification. The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.catchingUp.details': 'If this stays for about a quarter of an hour, the server administrator gets a notification. The backup DNS server must be switched on and reach this server on TCP port 53. The CelikPanel Agent’s lines about it are read with this command:',
} as const;

export type DeletionGuideScreenKey = keyof typeof enDeletionGuideScreens;
