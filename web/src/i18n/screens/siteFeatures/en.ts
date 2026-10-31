// SCREEN catalogue part "siteFeatures" (alpha.114): a domain's password-protected
// folders and redirects, the refusal codes of those two screens, and the
// two-factor recovery codes of the account page.
// Key prefixes kept here: siteFeatures., account.recovery., err.SITE_FEATURE_,
// err.SITE_AUTH_FILE_KEPT.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived. How to split
// a part that grows too large is in web/scripts/check-bundle-budget.mjs.
//
// "siteFeatures" ekran parçası (alpha.114): alan adının parolalı klasörleri ve
// yönlendirmeleri, bu iki ekranın ret kodları ve hesap sayfasının iki faktörlü
// kurtarma kodları.

export const enSiteFeaturesScreens = {
    // --- password-protected folders ---
    'siteFeatures.protect.title': 'Protected folders',
    'siteFeatures.protect.desc': 'Ask for a user name and a password before a folder of this site opens. nginx itself checks the password, so it keeps working even if CelikPanel is stopped.',
    'siteFeatures.protect.checking': 'Reading the protected folders from the server…',
    'siteFeatures.protect.unknown': 'The protected folders could not be read just now. Nothing was changed. Try again.',
    'siteFeatures.protect.unavailable': 'Protected folders are available for PHP and static sites. This site runs another kind of project, so there is nothing to set here.',
    'siteFeatures.protect.httpNote': 'This site also answers over plain HTTP, where a user name and password travel unencrypted. Turn on the HTTPS redirect in the SSL/TLS settings so visitors always sign in over HTTPS.',
    'siteFeatures.protect.empty': 'No folder is protected yet.',
    'siteFeatures.protect.addTitle': 'Protect a folder',
    'siteFeatures.protect.path': 'Folder',
    'siteFeatures.protect.pathHint': 'The folder’s address on the site, for example /private. The folder does not have to exist yet.',
    'siteFeatures.protect.realm': 'Sign-in title',
    'siteFeatures.protect.realmHint': 'Shown by the browser in the sign-in box. Optional.',
    'siteFeatures.protect.username': 'User name',
    'siteFeatures.protect.password': 'Password',
    'siteFeatures.protect.passwordHint': '8 to 72 characters. CelikPanel keeps only a hash; nobody can read the password back.',
    'siteFeatures.protect.add': 'Protect folder',
    'siteFeatures.protect.adding': 'Protecting…',
    'siteFeatures.protect.users': 'Users',
    'siteFeatures.protect.addUser': 'Add a user or change a password',
    'siteFeatures.protect.saveUser': 'Save user',
    'siteFeatures.protect.removeUser': 'Remove user {name}',
    'siteFeatures.protect.removeUserConfirm': 'Remove user {name} from {path}? This user can no longer open the folder.',
    'siteFeatures.protect.removeDir': 'Remove protection',
    'siteFeatures.protect.removeDirConfirm': 'Remove the password from {path}? Everybody can open the folder again.',
    'siteFeatures.protect.added': 'Done. nginx now asks for a password on {path}.',
    'siteFeatures.protect.userSaved': 'Saved. The user can open {path} with this password.',
    'siteFeatures.protect.userRemoved': 'Removed.',
    'siteFeatures.protect.dirRemoved': 'Done. {path} is open to everybody again.',
    'siteFeatures.protect.nginxNote': 'The change is applied only after nginx accepted the whole configuration; if it did not, nothing changed and you see why here.',
    // --- redirects ---
    'siteFeatures.redirect.title': 'Redirects',
    'siteFeatures.redirect.desc': 'Send visitors from an address of this site to another address. nginx does it itself, so it keeps working even if CelikPanel is stopped.',
    'siteFeatures.redirect.checking': 'Reading the redirects from the server…',
    'siteFeatures.redirect.unknown': 'The redirects could not be read just now. Nothing was changed. Try again.',
    'siteFeatures.redirect.unavailable': 'A forwarding site already sends every address to its target, so it has no redirects of its own.',
    'siteFeatures.redirect.empty': 'No redirect yet.',
    'siteFeatures.redirect.addTitle': 'Add a redirect',
    'siteFeatures.redirect.match': 'What is redirected',
    'siteFeatures.redirect.match.exact': 'This address only',
    'siteFeatures.redirect.match.prefix': 'This folder and everything below it',
    'siteFeatures.redirect.source': 'From (path on this site)',
    'siteFeatures.redirect.sourceHint': 'For example /old-page or /blog. A folder redirect keeps the rest of the address: /blog/post goes to the target followed by /post.',
    'siteFeatures.redirect.target': 'To',
    'siteFeatures.redirect.targetHint': 'A full address (https://example.org/new) or a path on this site (/new).',
    'siteFeatures.redirect.code': 'Kind',
    'siteFeatures.redirect.code.301': '301 – moved for good',
    'siteFeatures.redirect.code.302': '302 – moved for now',
    'siteFeatures.redirect.code.307': '307 – for now, same method',
    'siteFeatures.redirect.code.308': '308 – for good, same method',
    'siteFeatures.redirect.keepQuery': 'Keep the visitor’s query string (the part after ?)',
    'siteFeatures.redirect.add': 'Add redirect',
    'siteFeatures.redirect.adding': 'Adding…',
    'siteFeatures.redirect.remove': 'Remove the redirect from {path}',
    'siteFeatures.redirect.removeConfirm': 'Remove the redirect from {path}?',
    'siteFeatures.redirect.added': 'Done. {path} now redirects.',
    'siteFeatures.redirect.removed': 'Removed.',
    'siteFeatures.redirect.badge.exact': 'exact',
    'siteFeatures.redirect.badge.prefix': 'folder',
    'siteFeatures.redirect.badge.query': 'keeps ?query',
    // --- checks done before anything is sent ---
    'siteFeatures.check.path': 'The folder must start with a slash and use only letters, digits, dot, dash, underscore and tilde, for example /private.',
    'siteFeatures.check.username': 'The user name may contain only letters, digits, dot, dash and underscore.',
    'siteFeatures.check.password': 'The password must have 8 to 72 characters.',
    'siteFeatures.check.target': 'The target must be a path starting with / or a full http:// or https:// address, without spaces, quotes, semicolons, braces or #.',
    'siteFeatures.check.source': 'The path must start with a slash and use only letters, digits, dot, dash, underscore and tilde.',
    // --- refusals ---
    'err.SITE_FEATURE_INVALID': 'CelikPanel did not accept this input, so nothing was changed.',
    'err.SITE_FEATURE_DUPLICATE': 'This folder or path is already in the list, so nothing was changed. Remove the old entry first if you want to replace it.',
    'err.SITE_FEATURE_LIMIT': 'The list is full, so nothing was changed. Remove an entry you no longer need.',
    'err.SITE_FEATURE_NOT_FOUND': 'That entry is not there any more, so nothing was changed. The list is read again.',
    'err.SITE_FEATURE_LAST_USER': 'This is the last user of the folder, so nothing was changed. Remove the folder’s protection instead.',
    'err.SITE_FEATURE_UNAVAILABLE': 'This kind of site cannot carry this setting, so nothing was changed. Remove the entries first if you want to change the site type.',
    'err.SITE_AUTH_FILE_KEPT': 'A password file of this site was changed on the server outside CelikPanel, so CelikPanel kept it and did not apply this change. Nothing else was changed. The server administrator moves that file aside (it is in /etc/nginx/celikpanel-auth.d/); then make the change again.',
    // --- two-factor recovery codes (account page) ---
    'account.recovery.title': 'Recovery codes',
    'account.recovery.desc': 'Ten single-use codes. If you lose your authenticator, a code signs you in instead of the six-digit code. Keep them apart from your phone.',
    'account.recovery.remaining': '{remaining} of {total} codes left.',
    'account.recovery.low': 'Only {remaining} left. Make a new set soon.',
    'account.recovery.none': 'No code is left. Make a new set now, while your authenticator still works.',
    'account.recovery.unknown': 'The number of recovery codes could not be read just now. Nothing was changed.',
    'account.recovery.regenHint': 'To make a new set, enter your password and a current authenticator code. The old codes stop working at once.',
    'account.recovery.regenerate': 'Make new codes',
    'account.recovery.working': 'Making codes…',
    'account.recovery.done': 'A new set of recovery codes was made. The old ones no longer work.',
    'account.recovery.unreadable': 'The codes were made, but this page could not read them, and they cannot be shown again. Make a new set now; the old set no longer matters.',
    'account.recovery.shownTitle': 'Save these codes now',
    'account.recovery.shownOnce': 'They are shown only once and cannot be read again later. Each code works one time.',
    'account.recovery.copyAll': 'Copy all',
    'account.recovery.download': 'Download as a text file',
    'account.recovery.saved': 'I saved the codes',
    'account.recovery.fileHeader': 'CelikPanel two-factor recovery codes. Each code works once.',
} as const;

export type SiteFeaturesScreenKey = keyof typeof enSiteFeaturesScreens;
