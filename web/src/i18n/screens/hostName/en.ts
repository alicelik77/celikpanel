// SCREEN catalogue part "hostName" (alpha.121): this server's own host name
// when public DNS does not answer for it (the Dashboard notice), with the
// reviewed address record where this panel serves the zone. Key prefix kept
// here: hostDNS.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "hostName" ekran parçası: sunucunun kendi adı genel DNS'te çözülmüyorsa
// Ana sayfadaki bildirim ve gözden geçirilen adres kaydı.

export const enHostNameScreens = {
    'hostDNS.missing': 'This server’s host name {host} does not resolve in public DNS: no address record answers for it.',
    'hostDNS.other': 'This server’s host name {host} points to another address in public DNS, not to this server.',
    'hostDNS.zoneNext': 'This panel serves the {zone} zone. Who acts: you. Next: add the address record for this server; public DNS shows it within minutes.',
    'hostDNS.presentNext': 'The {zone} zone on this panel already has {records} for it, but public DNS does not answer with it yet. Usually the zone is not yet delegated to this server’s name servers, or the change is still on its way. Who acts: you, at the domain’s registrar. Next: check the name servers of {zone} there, then press Check again.',
    'hostDNS.externalNext': 'The DNS of {domain} is not served by this panel. Who acts: whoever manages that DNS. Next: add an address record {host} → {ip} there, then press Check again.',
    'hostDNS.otherNext': 'Public DNS answers {addresses}; this server is {ip}. Who acts: whoever manages that DNS. Next: if this is not intended, change the address record, then press Check again.',
    'hostDNS.why': 'Mail from this server and the certificate for this name need it to resolve. Nothing on the server was changed.',
    'hostDNS.add': 'Add the record…',
    'hostDNS.recheck': 'Check again',
    'hostDNS.openZone': 'Open the {zone} zone',
    'hostDNS.dialogTitle': 'Add the address record to {zone}',
    'hostDNS.dialogBody': 'These records will be added to the {zone} zone on this panel and published, so that {host} resolves to this server:',
    'hostDNS.dialogWait': 'Public resolvers may keep the old “does not exist” answer for a few minutes after this.',
    'hostDNS.confirm': 'Add and publish',
    'hostDNS.added': 'The record was added to {zone} and published. Public DNS shows {host} within minutes; press Check again then.',
    'hostDNS.addFailed': 'The record {record} could not be added to {zone}: {reason}',
    'hostDNS.addLost': 'The answer for {record} was lost, so it may or may not have been added. Nothing is sent again by itself: press Check again, or open the zone to see.',
} as const;

export type HostNameScreenKey = keyof typeof enHostNameScreens;
