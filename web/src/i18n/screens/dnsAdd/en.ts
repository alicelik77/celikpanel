// SCREEN catalogue part "dnsAdd" (alpha.135): adding a DNS record where the
// panel can do it safely - the paired main DNS server asked over the
// authorized connection, the steps and how each request ends, the wait for
// public DNS, and where a name's DNS is managed otherwise. Used first by the
// webmail address check (components/WebmailAddressCard.tsx,
// lib/webmailAddressRecords.ts). Key prefix: dnsadd.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "dnsAdd": DNS kaydını panelin güvenle ekleyebildiği
// yere ekleme. Önek: dnsadd.

export const enDnsAddScreens = {
    'dnsadd.next.peer': 'press Add to ask the other server {server} to add the record below (you see it before anything is sent), or add it there yourself. Then press Check.',
    'dnsadd.managedBy': 'This name’s DNS is managed on the name servers of {zone}: {nameservers}.',
    'dnsadd.addPeer': 'Add…',
    'dnsadd.peer.title': 'Ask {server} to add the record?',
    'dnsadd.peer.body': 'CelikPanel will ask the other server {server} to add exactly this to its zone {zone}:',
    'dnsadd.peer.bodyAnyZone': 'CelikPanel will ask the other server {server} to add exactly this to the zone that holds this name:',
    'dnsadd.peer.transfer': 'The record reaches this server by the normal DNS transfer.',
    'dnsadd.peer.aaaa': 'The AAAA record is added only if {server} sees the request come from this server’s IPv6 address; otherwise only the A record is added, and webmail works with it.',
    'dnsadd.peer.keptZone': 'Nothing else in the zone changes.',
    'dnsadd.peer.keptOwner': 'A record that already exists for this name on {server} is kept; the request is then refused.',
    'dnsadd.peer.confirm': 'Add',
    'dnsadd.peer.progress': 'Adding the record on {server}',
    'dnsadd.step.ask': 'Asking {server}',
    'dnsadd.step.write': 'The record is written on {server}',
    'dnsadd.step.writeZone': 'The record is written to the zone {zone}',
    'dnsadd.step.dns': 'Public DNS shows the record',
    'dnsadd.step.dnsNote': 'This continues on the page: it looks every 20 seconds, for at most 5 minutes. You don’t need to do anything.',
    'dnsadd.step.noAnswer': 'No answer came.',
    'dnsadd.doneTitle': 'Record added',
    'dnsadd.notAddedTitle': 'The record was not added',
    'dnsadd.unknownTitle': 'Whether the record was added is not known',
    'dnsadd.peer.added': '{server} added {record}.',
    'dnsadd.peer.present': '{server} already had {record}; nothing needed to change.',
    'dnsadd.peer.saved': '{server} wrote {record} and has not confirmed publishing it yet; public DNS will show it.',
    'dnsadd.peer.alsoAAAA': 'The AAAA record was added too.',
    'dnsadd.peer.unknown': 'No answer came from {server}, so whether the record was added is not known. It is not sent again by itself. Next: press Check to see what DNS holds now.',
    'dnsadd.peer.notOffered': 'This server can no longer ask {server} for this record (the DNS connection or the pair changed). Nothing was sent. Next: press Check.',
    'dnsadd.refused.outside_zone': '{server} does not hold this name in a zone it manages, so it added nothing. Nothing changed. Who acts: you. Next: add the record where this name’s DNS is managed (Copy), then press Check.',
    'dnsadd.refused.taken': 'On {server} this name already has another record, or its record was changed or removed by hand there; CelikPanel never overwrites it. Nothing changed. Who acts: you. Next: look at this name’s records in that server’s DNS editor and decide there, then press Check.',
    'dnsadd.refused.address': '{server} did not recognize this server’s address as the verified address of the DNS pair, so it added nothing. Nothing changed. Who acts: you. Next: add the record on {server} yourself (Copy) and review the pair’s addresses in Server setup.',
    'dnsadd.refused.invalid': '{server} did not accept the request as a host record. Nothing changed. Next: add the record yourself (Copy), then press Check.',
    'dnsadd.refused.not_ready': '{server}’s DNS is not ready right now (its link with this server could not be confirmed), so it added nothing. Nothing changed. Who acts: that server’s administrator. Next: look at its DNS page; when it is ready, press Add here again.',
    'dnsadd.refused.unauthorized': '{server} no longer accepts this server’s DNS connection (it was revoked or replaced). Nothing changed. Who acts: you. Next: connect the two servers again, or add the record yourself (Copy).',
    'dnsadd.refused.unsupported': '{server} does not take such requests (it may run an older version). Nothing changed. Next: add the record yourself (Copy), then press Check.',
    'dnsadd.refused.license': '{server}’s panel licence is not active or could not be checked, so the request was not handled. Nothing changed. Who acts: that server’s administrator. Next: check the licence there, or add the record yourself (Copy).',
    'dnsadd.refused.unreachable': '{server} could not be reached; the request did not leave this server. Nothing changed. Next: press Add again later, or add the record yourself (Copy).',
    'dnsadd.aaaa.address': 'The AAAA record was not added: {server} did not see this request come from this server’s IPv6 address. It is optional; webmail works with the A record. You can copy it and add it yourself:',
    'dnsadd.aaaa.unreachable': 'The AAAA record was not added: {server} could not be reached over this server’s IPv6 address. It is optional; webmail works with the A record. You can copy it and add it yourself:',
    'dnsadd.aaaa.other': 'The AAAA record was not added: {server} did not accept it. It is optional; webmail works with the A record. You can copy it and add it yourself:',
    'dnsadd.aaaa.unknown': 'No answer came for the AAAA record, so whether it was added is not known; Check shows what DNS holds. It is optional.',
    'dnsadd.wait': 'Waiting for public DNS to show the record. You don’t need to do anything: this page looks every 20 seconds, for at most 5 minutes.',
    'dnsadd.waitOver.zone': 'The record was added to the zone {zone}, but public DNS did not show it within 5 minutes. This is not an error: DNS can take longer to reach everyone. Next: press Check in a few minutes. It is not added again from here.',
    'dnsadd.waitOver.peer': 'The record was added on {server}, but public DNS did not show it within 5 minutes. This is not an error: DNS can take longer to reach everyone. Next: press Check in a few minutes. It is not added again from here.',
} as const;

export type DnsAddScreenKey = keyof typeof enDnsAddScreens;
