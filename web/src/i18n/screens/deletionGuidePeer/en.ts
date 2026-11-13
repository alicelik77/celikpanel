// SCREEN catalogue part "deletionGuidePeer" (alpha.133 round 5): the rest of
// the deletion guide (part "deletionGuide"), split out by size only - every
// other reason of the backup DNS server's confirmation said as why, who acts,
// the next step and how it continues. A backup that did not answer waits by
// itself; for the others a person fixes the cause on one of the two servers
// while CelikPanel keeps checking and finishes the deletion by itself. Key
// prefix: domains.deletionGuide. (shared with the part "deletionGuide").
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "deletionGuidePeer": yedek DNS sunucusunun
// onayını bekleyen silmenin diğer nedenleri; boyut nedeniyle ayrıldı.

export const enDeletionGuidePeerScreens = {
    'domains.deletionGuide.peerNoAnswer.why': 'The domain is removed from DNS on this server; the backup DNS server did not answer this server’s check (it may be switched off, restarting or not reachable), so its removal there is not confirmed yet.',
    'domains.deletionGuide.peerRefused.why': 'The domain is removed from DNS on this server; the backup DNS server answered this server’s check but did not confirm that it removed the domain too.',
    'domains.deletionGuide.peerSetup.why': 'The domain is removed from DNS on this server; whether the backup DNS server removed it too cannot be checked yet, because this server has no access set up to check it.',
    'domains.deletionGuide.peerAccessChanged.why': 'The domain is removed from DNS on this server; the access this server uses to check the backup DNS server has changed or cannot be trusted, so the removal there is not confirmed yet.',
    'domains.deletionGuide.peerNotRemoved.why': 'The domain is removed from DNS on this server, but the backup DNS server’s answer did not show that it removed the domain too.',
    'domains.deletionGuide.evidenceChanged.why': 'The domain is removed from DNS on this server, but the DNS of this server or of the backup DNS server changed while the removal was being confirmed, so CelikPanel does not confirm it yet.',
    'domains.deletionGuide.transferRefused.why': 'The domain is removed from DNS on this server; the backup DNS server did not let its own check read this server’s list of domains, so the removal there is not confirmed yet.',
    'domains.deletionGuide.internal.why': 'The domain is removed from DNS on this server, but this server could not run its own check of the backup DNS server; nobody changed anything and DNS answers are not affected.',
    'domains.deletionGuide.peerRefused.who': 'You, or whoever runs the backup DNS server.',
    'domains.deletionGuide.peerSetup.who': 'You, as this server’s administrator.',
    'domains.deletionGuide.peerAccessChanged.who': 'You, as this server’s administrator.',
    'domains.deletionGuide.peerNotRemoved.who': 'You, or whoever runs the backup DNS server.',
    'domains.deletionGuide.evidenceChanged.who': 'You, as this server’s administrator.',
    'domains.deletionGuide.transferRefused.who': 'You, or whoever runs the backup DNS server.',
    'domains.deletionGuide.internal.who': 'You, as this server’s administrator.',
    'domains.deletionGuide.peerRefused.next': 'On the backup DNS server, check that its DNS service is running and that it still allows this server’s check.',
    'domains.deletionGuide.peerSetup.next': 'Open Server setup on this server: it says why the access to the backup DNS server is not set up and how to finish it.',
    'domains.deletionGuide.peerAccessChanged.next': 'If the access between the two servers was changed by hand, set it up again; on two servers joined with a setup join code, Server setup on this server does it.',
    'domains.deletionGuide.peerNotRemoved.next': 'On the backup DNS server, check that its DNS service is running and has taken this server’s latest list of domains.',
    'domains.deletionGuide.evidenceChanged.next': 'If DNS was changed by hand on either server just now, check that change; otherwise nothing more is needed.',
    'domains.deletionGuide.transferRefused.next': 'On the backup DNS server, allow this transfer in its DNS settings (the exact setting is under Details in the waiting notice on Domains).',
    'domains.deletionGuide.internal.next': 'Send the CelikPanel Agent’s lines to CelikPanel support (the command is under Details in the waiting notice on Domains).',
    'domains.deletionGuide.then.answer.at': 'CelikPanel checks again by itself at about {time}; once the backup DNS server answers, the deletion finishes by itself, with nothing to press.',
    'domains.deletionGuide.then.answer.now': 'CelikPanel is checking again right now; once the backup DNS server answers, the deletion finishes by itself, with nothing to press.',
    'domains.deletionGuide.then.answer.soon': 'CelikPanel checks again by itself within a few minutes; once the backup DNS server answers, the deletion finishes by itself, with nothing to press.',
    'domains.deletionGuide.then.fix.at': 'CelikPanel checks again by itself at about {time}, then every few minutes; once this is fixed, the deletion finishes by itself, with nothing to press.',
    'domains.deletionGuide.then.fix.now': 'CelikPanel is checking again right now, then every few minutes; once this is fixed, the deletion finishes by itself, with nothing to press.',
    'domains.deletionGuide.then.fix.soon': 'CelikPanel checks again by itself within a few minutes, then every few minutes; once this is fixed, the deletion finishes by itself, with nothing to press.',
    'domains.deletionGuide.peerNoAnswer.details': 'This server checks the backup DNS server over SSH (port 22 unless it was changed): the connection was refused, did not come up or got no answer in time. If this lasts about a quarter of an hour, the server administrator gets a notification. The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.peerRefused.details': 'The backup DNS server’s check tool, reached over SSH with the enrolled key, answered without confirming the removal (another host key, the key refused, or the tool reported a reason). The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.peerSetup.details': 'On two CelikPanel servers joined with a setup join code, setup sets this access up by itself. Otherwise the administrators of both servers set it up once with the dns-peer-enroll owner tool (primary-prepare here, secondary-install and secondary-host-key on the backup, then primary-activate here). The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.peerAccessChanged.details': 'The peer identity or the key this server uses for the backup DNS server differs from what was enrolled, or another check enrollment appeared. The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.peerNotRemoved.details': 'The backup DNS server’s own DNS state did not show the deleted zone as absent (loaded zones or transfer state). The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.evidenceChanged.details': 'One of the comparisons CelikPanel makes while confirming the removal found different evidence; CelikPanel never overwrites such a change and checks again. The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.transferRefused.details': 'The backup DNS server’s named refused the local (loopback) transfer of this server’s catalog zone. Add 127.0.0.1 and ::1 to the catalog zone’s allow-transfer in named’s configuration on the backup and reload named; a backup set up by CelikPanel gets it when CelikPanel next writes its DNS configuration. The CelikPanel Agent’s lines about it are read with this command:',
    'domains.deletionGuide.internal.details': 'An internal precondition of this server’s check failed (for example its DNS engine record could not select the catalog checks); checking again does not help until a CelikPanel update fixes it, and after that update the deletion finishes by itself. The CelikPanel Agent’s lines about it are read with this command:',
} as const;

export type DeletionGuidePeerScreenKey = keyof typeof enDeletionGuidePeerScreens;
