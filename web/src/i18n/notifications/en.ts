// Route-scoped copy of the notifications (alpha.128, G18), like
// ../monitoringAlerts: the one list of the signed-in person's notices
// (Dashboard and My account), their email choice and the administrator's
// server-wide settings. It ships with those lazy chunks, so the critical boot
// path does not grow; ../notifications.ts looks keys up in the
// I18nProvider's locale. An account notice's own words come from the server
// in both languages (its text registry), not from here.
//
// Bildirimlerin kendi metni: tembel paketlerle gelir, açılış yolu büyümez.
// Hesap bildiriminin kendi sözleri sunucudan iki dilde gelir.

export const enNotifications = {
    'notif.title': 'Notifications',
    'notif.needsAction': 'Needs your action',
    'notif.forInfo': 'For your information',
    'notif.checking': 'Reading your notifications…',
    'notif.readFailed': 'Your notifications could not be read, so none are shown. This does not mean there are none. Nothing was changed.',
    'notif.whoActsYou': 'Who acts: you.',
    'notif.whoActsOther': 'Nothing for you to do: whoever acts was told too.',
    'notif.open': 'Open the page to fix it',
    'notif.openInfo': 'Open',
    'notif.seen': 'Mark as seen',
    'notif.seenAll': 'Mark all as seen',
    'notif.seenFailed': 'It could not be marked as seen. It stays in the list; try again.',
    'notif.alerts': '{n} monitoring alerts are open on this server.',
    'notif.alertsOne': '1 monitoring alert is open on this server.',
    'notif.alertsOpen': 'Open Monitoring',
    'notif.emailedNote': 'These are also emailed as you chose under My account → Notifications.',

    'notif.prefs.title': 'Notifications by email',
    'notif.prefs.desc': 'Every notice is always shown in the panel. Choose which ones are also emailed to you.',
    'notif.prefs.all': 'Everything',
    'notif.prefs.allHint': 'Every notice about your account and what you manage.',
    'notif.prefs.action': 'Only what needs my action',
    'notif.prefs.actionHint': 'Only notices where you are the one who acts (for example, a limit to raise or an end date to extend).',
    'notif.prefs.none': 'Nothing',
    'notif.prefs.noneHint': 'No email. You see the notices only here in the panel.',
    'notif.prefs.extra': 'Also send to (optional)',
    'notif.prefs.extraHint': 'One more address that receives the same email, for example a shared support desk. Leave empty for none.',
    'notif.badEmail': 'Write a whole email address, or leave the field empty.',
    'notif.prefs.saved': 'Your email choice was saved.',
    'notif.prefs.failed': 'Your email choice could not be saved, so nothing changed.',
    'notif.prefs.checking': 'Reading your email choice…',
    'notif.prefs.readFailed': 'Your email choice could not be read. Nothing was changed; try again.',
    'notif.prefs.noOwnEmail': 'Your account has no email address, so only the extra address (if any) receives email. Add your address in the Email section on this page.',
    'notif.prefs.noMail': 'This server cannot send email yet, so notices are shown only in the panel. Who acts: the administrator sets up mail; email starts by itself after that.',
    'notif.prefs.impersonating': 'You are browsing as this person: their email choice is theirs to change, so it is shown but cannot be saved here.',

    'notif.admin.title': 'Server-wide notification settings',
    'notif.admin.desc': 'For the whole server. Each person still chooses their own email above.',
    'notif.admin.expiry': 'Warn before a subscription ends',
    'notif.admin.expiryHint': 'The customer, their reseller and you are told this many days before the end date, and again within the last day. On the end date every service of the subscription is suspended; nothing is deleted.',
    'notif.admin.days': '{n} days before',
    'notif.admin.extraTitle': 'Extra address per kind of event (optional)',
    'notif.admin.extraHint': 'One copy of each event goes to this address, whatever the people chose. Leave empty for none.',
    'notif.admin.group.monitoring': 'Monitoring alerts',
    'notif.admin.group.account': 'Account and usage notices',
    'notif.admin.group.security': 'Security events',
    'notif.admin.saved': 'The notification settings were saved.',
    'notif.admin.failed': 'The notification settings could not be saved, so nothing changed.',
    'notif.admin.checking': 'Reading the notification settings…',
    'notif.admin.readFailed': 'The notification settings could not be read. Nothing was changed; try again.',
} as const;

export type NotificationsKey = keyof typeof enNotifications;
