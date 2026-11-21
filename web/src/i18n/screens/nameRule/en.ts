// SCREEN catalogue part "nameRule" (security release, 20 November 2026): the
// refusal of a name another account or the server holds and of a Node.js
// port that is not a site's (err.HOSTNAME_NOT_AVAILABLE, err.APP_PORT_NOT_ALLOWED).
// An ordinary account is told the same whoever holds a name; only the
// administrator is told the reason and the other name. A new part, as the
// "errors" part is full.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "nameRule" ekran parçası: tek ad kuralının ve Node.js port kuralının retleri.

export const enNameRuleScreens = {
    'err.HOSTNAME_NOT_AVAILABLE': '{name} cannot be added here: this name, a name above it or a name under it is already in use on this server or reserved by it. Nothing was changed. If the name is yours, ask the server’s administrator to check it. A subdomain of one of your own domains can always be added.',
    'err.HOSTNAME_NOT_AVAILABLE.own_name': 'You already have {name}: it is one of your domains, its www or mail name, or an alias. Nothing was changed.',
    'err.HOSTNAME_NOT_AVAILABLE.other_account': '{name} meets {other} of the account {holder}: a customer’s or reseller’s subscription never holds a name equal to, above or under another account’s name. Nothing was changed. Add it to the account that holds {other}, or to your own subscription.',
    'err.HOSTNAME_NOT_AVAILABLE.server_name': '{name} meets {other}, one of this server’s own names: only an administrator’s own subscription may hold it. Nothing was changed.',
    'err.HOSTNAME_NOT_AVAILABLE.remote_zone': '{name} meets {other}, a zone this server publishes for a connected server. Nothing was changed. Use that zone on the server that publishes it.',
    'err.HOSTNAME_NOT_AVAILABLE.account_taken': 'A site of {name} would run as the system account {account}, which another site or the system already has. Nothing was changed. Choose another name, or remove what uses that account first.',
    'err.APP_PORT_NOT_ALLOWED': 'This port cannot be the site’s application port. A Node.js site gets a free port from {min} to {max} that no other site or service of this server uses. Nothing was changed. Leave the port empty and the panel picks a free one.',
} as const;

export type NameRuleScreenKey = keyof typeof enNameRuleScreens;
