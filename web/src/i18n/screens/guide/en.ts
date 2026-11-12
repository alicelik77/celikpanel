// SCREEN catalogue part "guide" (alpha.119): the user guide's screens - the
// help menu behind "?", the guide drawer, the /help page and the first-login
// tour. The chapters themselves are not here: they are generated from
// docs/guide into web/src/help/guide/gen.
// Key prefixes kept here: guide.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "guide" ekran parçası (alpha.119): kılavuzun ekranları ve ilk giriş turu.
// Bölümler burada değil; docs/guide'dan üretilir.

export const enGuideScreens = {
    'guide.title': 'User guide',
    'guide.subtitle': 'How to use CelikPanel, chapter by chapter. The same guide is on celikpanel.net.',
    'guide.button': 'Help',
    'guide.menu.page': 'Help for this page',
    'guide.menu.guide': 'User guide',
    'guide.menu.tour': 'Take the tour',
    'guide.loading': 'Opening the guide…',
    'guide.unknown': 'The guide could not be opened just now. Nothing else is affected. Try again.',
    'guide.noTopic': 'This page has no chapter of its own yet, so the guide opens at its start.',
    'guide.chapters': 'Chapters',
    'guide.notInGuide': 'That chapter is not part of this guide, so the guide opens at its start.',
    'guide.close': 'Close the guide',
    'guide.openFull': 'Open in the full guide',
    'guide.viewAs': 'Show the guide for',
    'guide.viewAsHint': 'You see the administrator’s chapters. Choose a role to read what your resellers and customers read.',
    'guide.search.label': 'Search the guide',
    'guide.search.placeholder': 'Search, for example: SSH',
    'guide.search.none': 'Nothing in this guide matches “{query}”. Try another word, or choose a chapter.',
    'guide.search.count': '{count} results',
    'guide.search.text': 'Searching the text of every chapter…',
    'guide.search.partial': 'The text of some chapters could not be fetched, so they are found by their titles and headings only. Search again to retry.',
    'guide.note': 'Note',
    'guide.warning': 'Warning',
    'guide.table': 'Table: {name}',
    'guide.external': '(opens a new tab)',
    'guide.link': 'How this works',

    'guide.tour.label': 'Tour of CelikPanel',
    'guide.tour.progress': 'Step {step} of {total}',
    'guide.tour.skip': 'Skip the tour',
    'guide.tour.back': 'Back',
    'guide.tour.next': 'Next',
    'guide.tour.finish': 'Finish',
    'guide.tour.saveFailed': 'Your choice could not be saved, so the tour may be offered again when you next sign in. Nothing else is affected.',
    'guide.tour.offer.title': 'The tour has new steps',
    'guide.tour.offer.text': 'Some screens changed since you took the tour. It takes a minute, and you can stop at any step.',
    'guide.tour.offer.take': 'Take the tour',
    'guide.tour.offer.dismiss': 'Not now',

    'guide.tour.welcome.title': 'Welcome to CelikPanel',
    'guide.tour.welcome.text': 'This short tour shows where things are. You can skip it at any step, and take it again later from the ? menu at the top right.',
    'guide.tour.setup.title': 'Server setup',
    'guide.tour.setup.text': 'Finish setting up the server here: what it is for, its name and what it should run. The page says what is still missing and why.',
    'guide.tour.components.title': 'Components',
    'guide.tour.components.text': 'Install and look after what the server runs: the web server, PHP, mail, DNS and databases. Every component has its own page.',
    'guide.tour.domains.title': 'Domains',
    'guide.tour.domains.text': 'Every website, mailbox and DNS zone belongs to a domain. Open a domain to see its sections: site, files, mail, databases, DNS and more.',
    'guide.tour.users.title': 'Users',
    'guide.tour.users.text': 'Create your plans and your customers here. What a customer may use comes from the plan you give them.',
    'guide.tour.dashboard.title': 'Dashboard',
    'guide.tour.dashboard.text': 'An overview of your sites and of anything that needs your attention.',
    'guide.tour.databases.title': 'Databases',
    'guide.tour.databases.text': 'Create databases and their users for your sites.',
    'guide.tour.help.title': 'Help on every page',
    'guide.tour.help.text': 'Press ? on any page to read the part of the guide that explains it. The full guide and this tour are in the same menu.',
    'guide.tour.account.title': 'Your menu',
    'guide.tour.account.text': 'Your account: your password, your email address and two-factor authentication. Sign out is here too.',
    'guide.tour.accountUser.text': 'Change your password and your email address here, and sign out.',
} as const;

export type GuideScreenKey = keyof typeof enGuideScreens;
