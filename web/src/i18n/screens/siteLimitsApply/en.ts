// SCREEN catalogue part "siteLimitsApply" (22 Nov 2026, S4 R2a second fix
// round): the limit in force on a site's card when it is not the plan's, a
// move that ran out of time, the preview of a lowered site limit, and the
// Problems now lines of a limit change that did not reach every site and of
// a default memory limit set for the server's earlier memory (pnow.). A new
// part beside "siteLimits", which has no room.
//
// EKRAN katalogu parçası "siteLimitsApply": kartta geçerli sınır, süresi
// dolan taşıma, düşürülen sınırın önizlemesi ve iki Şu anki sorunlar satırı.

export const enSiteLimitsApplyScreens = {
    'slim.applied': 'In force now: {value}.',
    'slim.move.timeUp': 'The move’s time ran out. The programs marked done are inside the limits; press the move again to continue with the rest.',
    'limits.preview.sitesStopped': 'The programs of {n} sites use more now than a lowered limit. On saving, a program over the memory limit is stopped at once; at the process limit no new process starts and running ones go on:',
    'pnow.site_limits_unapplied.title': 'New site limits have not reached {value} sites yet',
    'pnow.site_limits_unapplied.body': 'After a change of limits ({domain}), {value} sites still have their earlier limits ({other}). CelikPanel tries again every hour, and each site’s card shows the limits in force. This line goes away when every site has the new ones.',
    'pnow.site_limits_unapplied.action': 'Open Plans',
    'pnow.site_memory_default.title': 'The server’s memory changed: the sites’ default memory limit is still {domain} MB',
    'pnow.site_memory_default.body': 'It was set for the server’s memory at the time, which changed ({other}). Nothing changes by itself. The new default would be {value} MB; applying it first shows the sites that use more now.',
    'pnow.site_memory_default.action': 'Apply the new default',
    'pnow.site_memory_default.confirm': 'Sites without a memory limit of their own get {value} MB from now on.',
    'pnow.site_memory_default.step': 'Applying the new default',
    'pnow.site_memory_default.done': 'The new default applies. The sites get it in the background.',
    'pnow.site_memory_default.failed': 'The new default was not applied.',
};

export type SiteLimitsApplyScreenKey = keyof typeof enSiteLimitsApplyScreens;
