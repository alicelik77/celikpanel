// SCREEN catalogue part "mailWork" (alpha.128): mailbox work (empty a
// mailbox, aliases under a mailbox, sending reopened after a password change)
// and the action log words of the outgoing mail limits and plan mail settings.
// A part of its own so the dashboard and mail parts keep their budgets.
export const enMailWorkScreens = {
    'audit.resource.mail_outgoing': 'Outgoing mail limits',
    'dashboard.audit.action.mail.outgoing.mailbox': 'changed a mailbox’s hourly sending limit',
    'dashboard.audit.action.mail.outgoing.settings': 'changed the outgoing mail limits',
    'dashboard.audit.action.mail.plan.limits': 'changed a plan’s mail limits',
    'dashboard.audit.action.mail.account.empty': 'emptied a mailbox',
    'mail.empty.title': 'Empty {name}',
    'mail.empty.confirm': 'Every message in every folder of {name} is removed for good. The mailbox, its password, folders, aliases and settings stay, and mail keeps arriving. There is no undo: keep a copy in a mail program first if anything is still needed.',
    'mail.empty.confirmLabel': 'Remove all messages',
    'mail.empty.progress': 'Emptying {name}',
    'mail.empty.done': 'The mailbox is empty.',
    'mail.empty.doneCount': 'The mailbox is empty: {n} messages were removed.',
    'mail.aliases.label': 'Also receives:',
    'mail.aliases.add': 'Add an alias',
    'opstep.mailEmpty.check': 'Checking the mailbox',
    'opstep.mailEmpty.empty': 'Removing the messages of {name} (Dovecot)',
    'mail.sendingResumed': 'Sending from it, stopped for security, is open again.',
} as const;

export type MailWorkScreenKey = keyof typeof enMailWorkScreens;
