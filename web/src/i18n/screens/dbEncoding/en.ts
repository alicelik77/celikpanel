// SCREEN catalogue part "dbEncoding" (20 Nov 2026): a PostgreSQL database's
// text encoding on its card, and the one warning for a database that is not
// UTF-8 with its way out through the card's own buttons. Key prefix: dbenc.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "dbEncoding": bir PostgreSQL veritabanının metin
// kodlaması ve UTF-8 olmayan veritabanının tek uyarısı.

export const enDbEncodingScreens = {
    'dbenc.fact': 'Text: {encoding}',
    'dbenc.title.unchecked': 'Text in this database is stored unchecked ({encoding})',
    'dbenc.title.limited': 'This database cannot hold every language’s letters ({encoding})',
    'dbenc.why.unchecked': 'Letters outside English (ç, ğ, ı, ş and every other language’s) are kept as raw bytes without a check, so sorting, searching and moving it to another server can go wrong.',
    'dbenc.why.limited': 'It was created with the {encoding} encoding, which has no room for many letters, Turkish ğ, ı and ş among them.',
    'dbenc.who.manager': 'Whoever manages this domain’s databases',
    'dbenc.next': 'If it holds nothing yet, delete it and create it again. If it holds data, press Copy on this card: the copy is made as UTF-8 with all the data. Point your site’s database settings to the copy, check the site, then delete this database.',
    'dbenc.then': 'The copy checks every letter. If it stops at text that is not valid UTF-8, nothing changes and this database stays as it is; that text has to be converted to UTF-8 first. New databases are always created as UTF-8.',
};

export type DbEncodingScreenKey = keyof typeof enDbEncodingScreens;
