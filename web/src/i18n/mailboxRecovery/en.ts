// Route-scoped copy of mailbox recovery (alpha.130, like ../panelSessions):
// the recovery address dialog on the mailbox list and the public pages a
// mailbox's recovery links open. It ships with those lazy chunks, so the
// critical boot path does not grow; ../mailboxRecovery.ts looks keys up in
// the I18nProvider's locale.
//
// Posta kutusu kurtarmanın kendi metni: tembel paketlerle gelir.

export const enMailboxRecovery = {
    // The manager's dialog (mailbox list).
    'mrec.title': 'Recovery address',
    'mrec.mailbox': 'Mailbox',
    'mrec.what': 'If the person using this mailbox forgets its password, a reset link can be sent to this address. Whoever reads mail at this address can change the mailbox password, so use an address the person alone reads, at another provider or another mailbox.',
    'mrec.checking': 'Reading the recovery address…',
    'mrec.readFailed': 'The recovery address could not be read, so nothing is shown. This does not mean there is none. Nothing was changed.',
    'mrec.state.none': 'This mailbox has no recovery address. A forgotten password can only be changed here, by you.',
    'mrec.state.pending': 'Waiting for confirmation: a link was mailed to {address} {time}. The address works only after the link was opened (within 24 hours). Who acts: the person who reads {address}.',
    'mrec.state.confirmed': 'Active: {address}, confirmed {time}. The person can ask for a reset link on the page below.',
    'mrec.state.void.transferred': 'Not active any more: {address} was set before the domain moved to another subscription or owner. Set it again (or another address) to use recovery.',
    'mrec.state.void.local_domain_changed': 'Not active any more: the domain of {address} was removed from this server and added again, so the address may now belong to someone else. Set it again only if it is still the right person.',
    'mrec.address': 'Recovery address',
    'mrec.addressHint': 'An address the person reads, not this mailbox itself.',
    'mrec.send': 'Save and send confirmation',
    'mrec.resend': 'Send confirmation again',
    'mrec.sending': 'Sending…',
    'mrec.remove': 'Remove recovery address',
    'mrec.removing': 'Removing the recovery address',
    'mrec.removeConfirmTitle': 'Remove the recovery address of {address}?',
    'mrec.removeConfirmBody': 'Reset links will no longer be sent for this mailbox, and any link already sent stops working. The mailbox and its password do not change.',
    'mrec.removed': 'The recovery address was removed. Links already sent no longer work.',
    'mrec.nothingToRemove': 'This mailbox had no recovery address any more, so nothing was changed.',
    'mrec.sent': 'Saved. A confirmation link was handed to the mail system for {address}; the address works after it is opened.',
    'mrec.unchanged': 'This address is already the confirmed recovery address, so nothing was changed and no message was sent.',
    'mrec.requestPage': 'Page for a forgotten mailbox password:',
    'mrec.copyPage': 'Copy the page address',
    'mrec.noMail.admin': 'This server cannot send mail for the panel, so a recovery address cannot be confirmed. Make the server\'s mail service answer on this server, then come back.',
    'mrec.noMail.other': 'This server cannot send mail for the panel, so a recovery address cannot be confirmed. Ask your provider to fix the server\'s mail service.',
    'mrec.close': 'Close',
    'mrec.failed': 'The recovery address could not be saved.',
    'mrec.removeFailed': 'The recovery address could not be removed.',

    // The public pages.
    'mrec.page.requestTitle': 'Forgot your mailbox password?',
    'mrec.page.requestBody': 'Type the mailbox address. If it has a confirmed recovery address, a reset link is sent there. It works for 30 minutes.',
    'mrec.page.mailbox': 'Mailbox address',
    'mrec.page.requestSend': 'Send reset link',
    'mrec.page.requestSending': 'Sending…',
    'mrec.page.requestDone': 'If this mailbox has a confirmed recovery address, a reset link is on its way there. Check that inbox (and its spam folder). If nothing arrives, the mailbox may have no recovery address: ask whoever manages your mail to change the password.',
    'mrec.page.confirmTitle': 'Confirm the recovery address',
    'mrec.page.confirming': 'Confirming the address…',
    'mrec.page.confirmDone': 'Confirmed. If the password of {mailbox} is forgotten, a reset link can now be sent to this address. The mailbox was told.',
    'mrec.page.resetTitle': 'Choose a new mailbox password',
    'mrec.page.resetFor': 'Mailbox: {mailbox}',
    'mrec.page.password': 'New password',
    'mrec.page.repeat': 'Repeat the new password',
    'mrec.page.mismatch': 'The two passwords are not the same.',
    'mrec.page.resetHint': 'After saving, mail programs that are signed in to this mailbox are disconnected and ask for the new password. The mailbox is told about the change.',
    'mrec.page.save': 'Save password',
    'mrec.page.saving': 'Saving…',
    'mrec.page.resetDone': 'The mailbox password is changed. Sign in to your mail with the new password.',
    'mrec.page.sessionsNotEnded': 'Mail programs that were already signed in could not be disconnected; they stop when they connect again.',
    'mrec.page.sendingPaused': 'Sending from this mailbox stays paused by your provider. Ask them to open it again.',
    'mrec.page.sendingResumed': 'Sending from this mailbox, paused for security, is open again.',
    'mrec.page.sendingResumePending': 'Sending from this mailbox, paused for security, opens again within a few minutes.',
    'mrec.page.missing': 'This page needs the link from your email. Open the link exactly as it arrived; if it was cut, ask for a new one.',
    'mrec.page.unknown': 'The answer did not arrive, so it is not known whether the change was saved. Try signing in to your mail with the new password; if that fails, ask for a new link.',
    'mrec.page.newLink': 'Ask for a new link',

    // Refusals (the server's codes).
    'err.mailbox_recovery_address_invalid': 'The recovery address is not a valid email address, so nothing was changed. Type a complete address such as name@example.com.',
    'err.mailbox_recovery_address_self': 'A mailbox cannot be its own recovery address, so nothing was changed. Use an address at another mailbox the person reads.',
    'err.mailbox_recovery_too_many': 'Too many confirmation messages for this mailbox or from your account, so nothing was sent. Wait an hour, then try again.',
    'err.mailbox_recovery_link_invalid': 'This link is not valid any more: it was used, replaced by a newer one, expired, or the mailbox changed. Nothing was changed. Ask for a new link.',
    'err.mailbox_recovery_login_disabled': 'This mailbox cannot sign in at the moment (it is suspended or turned off), so its password was not changed. Ask your provider.',
    'err.mailbox_recovery_result_unknown': 'It is not known whether the new password was set: the server did not answer in time. Try signing in with the new password; if it does not work, ask for a new link.',
    'err.mailbox_recovery_busy': 'Another mail change is running on the server, so the password was not changed and the link still works. Try again in a minute.',
    'err.mailbox_recovery_mail_not_sent': 'The recovery address was saved, but the server\'s mail system did not accept the confirmation message, so it was not sent. Press Send again in a few minutes; if it keeps failing, the administrator checks the mail service.',
};

export type MailboxRecoveryKey = keyof typeof enMailboxRecovery;
