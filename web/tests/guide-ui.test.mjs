// The user guide in the panel (alpha.119): which chapter a page opens, the
// first-login tour's decision and steps, the search, and how the shell
// carries it (lazy, after the screen copy, anchored on data-tour).
//
// Paneldeki kılavuz: sayfanın açtığı bölüm, turun kararı ve adımları, arama ve
// kabuğun onu nasıl taşıdığı.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource, dataModule } from './fixtures/shared-layer.mjs';
import { englishScreens, turkishScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const routerCoreURL = compileSource('router-core.ts', () => { throw new Error('router-core imports nothing'); });
const routes = await import(compileSource('help/guide/routes.ts', (s) => (s.endsWith('/router-core') ? routerCoreURL : dataModule(''))));
const tour = await import(new URL('../src/help/guide/tourState.ts', import.meta.url).href);
const search = await import(new URL('../src/help/guide/search.ts', import.meta.url).href);
const chunk = async (role, locale) => (await import(new URL(`../src/help/guide/gen/${role}.${locale}.ts`, import.meta.url).href)).default;

test('a route names its topic; an address the guide does not know names none', () => {
    assert.equal(routes.topicForPath('/'), 'dashboard');
    assert.equal(routes.topicForPath('/domains/example.com'), 'domains');
    assert.equal(routes.topicForPath('/domains/infrastructure/example.com'), 'dns.infrastructure');
    assert.equal(routes.topicForPath('/services/nginx'), 'components');
    assert.equal(routes.topicForPath('/help/files-access'), 'help');
    assert.equal(routes.topicForPath('/account/'), 'account');
    assert.equal(routes.topicForPath('/nowhere'), null);
});

test('the page\'s own topic wins, then its route, then the first chapter', async () => {
    const customer = await chunk('customer', 'en');
    // A domain's Access tab publishes domain.access: the files chapter at FTP.
    assert.deepEqual(routes.placeFor(customer, ['domain.access', 'domains']), { chapter: 'files-access', anchor: 'ftp-sftp', fallback: false });
    assert.deepEqual(routes.placeFor(customer, [null, 'dashboard']), { chapter: 'getting-started', anchor: '', fallback: false });
    assert.deepEqual(routes.placeFor(customer, [null, 'account']), { chapter: 'getting-started', anchor: 'your-account', fallback: false });
    // No chapter claims the topic yet: the guide's start, marked as such.
    assert.deepEqual(routes.placeFor(customer, ['domain.mail', 'domains']), { chapter: 'getting-started', anchor: '', fallback: true });
    // The administrator's guide has no files chapter (it is the customers').
    const admin = await chunk('admin', 'en');
    assert.equal(routes.placeFor(admin, ['domain.access']).fallback, true);
    assert.ok(!admin.chapters.some((c) => c.id === 'files-access'));
});

test('every page topic is a well-formed name, and every chunk claims the same topics in both languages', async () => {
    for (const route of routes.guideRoutes) assert.match(route.topic, /^[a-z0-9]+(?:[.-][a-z0-9]+)*$/, route.pattern);
    for (const role of ['admin', 'reseller', 'customer']) {
        const en = await chunk(role, 'en');
        const tr = await chunk(role, 'tr');
        assert.deepEqual(en.topics, tr.topics, role);
        assert.deepEqual(en.chapters.map((c) => c.id), tr.chapters.map((c) => c.id), role);
        assert.equal(en.role, role);
        assert.equal(tr.locale, 'tr');
    }
});

test('the tour is shown only for a known pending state, offered after a new version, never forced', () => {
    const { tourDecision, decodeGuideState, TOUR_VERSION } = tour;
    assert.equal(TOUR_VERSION, 1);
    assert.equal(tourDecision({ tour_version: 0, tour_state: 'pending' }, false), 'show');
    assert.equal(tourDecision({ tour_version: 1, tour_state: 'completed' }, false), 'none');
    assert.equal(tourDecision({ tour_version: 1, tour_state: 'skipped' }, false), 'none');
    assert.equal(tourDecision({ tour_version: 1, tour_state: 'skipped' }, false, 2), 'offer');
    assert.equal(tourDecision({ tour_version: 1, tour_state: 'completed' }, false, 2), 'offer');
    // Unknown state: nobody is interrupted. Log in as: the person's tour is kept.
    assert.equal(tourDecision(null, false), 'none');
    assert.equal(tourDecision({ tour_version: 0, tour_state: 'pending' }, true), 'none');
    assert.deepEqual(decodeGuideState({ tour_version: 2, tour_state: 'skipped' }), { tour_version: 2, tour_state: 'skipped' });
    for (const bad of [null, {}, { tour_version: '1', tour_state: 'pending' }, { tour_version: 1.5, tour_state: 'pending' }, { tour_version: 1, tour_state: 'done' }]) {
        assert.equal(decodeGuideState(bad), null, JSON.stringify(bad));
    }
});

test('each role has a tour of four to six steps; steps whose element is off screen are left out', () => {
    for (const role of ['admin', 'reseller', 'customer', 'additional_user']) {
        const steps = tour.tourSteps(role);
        assert.ok(steps.length >= 4 && steps.length <= 6, `${role}: ${steps.length}`);
        assert.equal(steps[0].anchor, undefined, `${role} starts with a welcome that points at nothing`);
        for (const step of steps) {
            for (const key of [step.title, step.text]) {
                assert.ok(englishScreens.includes(`'${key}':`), `${key} in English`);
                assert.ok(turkishScreens.includes(`'${key}':`), `${key} in Turkish`);
            }
        }
        // On a phone the rail is hidden: only the steps that need no menu item stay.
        const phone = tour.visibleSteps(steps, (anchor) => anchor === 'help' || anchor === 'account');
        assert.deepEqual(phone.map((s) => s.anchor ?? 'welcome'), ['welcome', 'help', 'account'], role);
    }
    assert.deepEqual(tour.tourSteps('admin').map((s) => s.anchor ?? 'welcome'), ['welcome', 'nav-serverSetup', 'nav-services', 'nav-domains', 'help', 'account']);
    assert.equal(tour.tourSteps('additional_user').at(-1).text, 'guide.tour.accountUser.text');
});

test('search folds Turkish letters and matches the start of words', async () => {
    assert.equal(search.fold('ŞİFRE Değiştir, IŞIK'), 'sifre degistir isik');
    const tr = await chunk('customer', 'tr');
    const hits = search.searchGuide(tr, 'ssh anahtar');
    assert.ok(hits.length > 0);
    assert.equal(tr.chapters[hits[0].chapter].id, 'files-access');
    assert.equal(hits[0].anchor, 'keys');
    // "dosya" finds "dosyaları"; an empty query finds nothing.
    assert.ok(search.searchGuide(tr, 'dosya').length > 0);
    assert.deepEqual(search.searchGuide(tr, '  '), []);
    assert.deepEqual(search.searchGuide(tr, 'zzzqqq'), []);
});

test('the shell carries the guide lazily, after the screen copy, with tour anchors', () => {
    const layout = read('../src/components/Layout.tsx');
    assert.match(layout, /const helpReady = !setupMode && screensReady && !screensFailed;/);
    // A part that cannot be fetched leaves the header without "?", never the shell broken.
    assert.match(layout, /import\('\.\.\/help\/guide\/HelpControl'\)\.then\(\(module\) => \{[\s\S]{0,160}\}, \(\) => undefined\);/);
    assert.match(layout, /\{helpReady && \(HelpControl \? <HelpControl \/> : <span className="h-9 w-9 max-sm:h-11 max-sm:w-11" aria-hidden="true" \/>\)\}/);
    assert.match(layout, /data-tour=\{`nav-\$\{item\.id\}`\}/);
    assert.match(layout, /data-tour="account"/);
    // The shell imports none of the guide's data or screens statically.
    assert.doesNotMatch(layout, /from '\.\.\/help\/guide\//);
    const control = read('../src/help/guide/HelpControl.tsx');
    assert.match(control, /data-tour="help"/);
    assert.match(control, /const GuideDrawer = lazy\(/);
    assert.match(control, /const Tour = lazy\(/);
    const nav = read('../src/nav.ts');
    assert.match(nav, /\{ id: 'help', path: '\/help', labelKey: 'nav\.help', icon: ScrollText, roles: DOMAIN_ROLES, group: 'personal' \}/);
    const app = read('../src/App.tsx');
    assert.match(app, /<Route path="\/help" element=\{<PageWithLayout><GuidePage \/><\/PageWithLayout>\} \/>/);
    assert.match(app, /<Route path="\/help\/:chapter" element=\{<PageWithLayout><GuidePage \/><\/PageWithLayout>\} \/>/);
    assert.match(read('../src/components/DomainDetail.tsx'), /useEffect\(\(\) => publishGuideTopic\(`domain\.\$\{activeTab\}`\), \[activeTab\]\);/);
    // The service help texts are untouched (their budget is nearly used).
    assert.doesNotMatch(read('../src/help/serviceHelp.ts'), /guide/i);
});

test('finish and skip are both saved once; Log in as saves nothing; a failed save says so', () => {
    const control = read('../src/help/guide/HelpControl.tsx');
    assert.match(control, /onFinish=\{\(\) => save\('completed'\)\} onSkip=\{\(\) => save\('skipped'\)\}/);
    assert.match(control, /onDismiss=\{\(\) => save\('skipped'\)\}/);
    const save = control.slice(control.indexOf('const save = useCallback'), control.indexOf('const itemClass'));
    assert.match(save, /decided\.set\(user\.username, Promise\.resolve\('none'\)\)/);
    assert.match(save, /if \(impersonating\) return;/);
    assert.match(save, /saveGuideState\(TOUR_VERSION, state\)/);
    assert.match(save, /if \(!saved\) showToast\('warning', t\('guide\.tour\.saveFailed'\)\)/);
    // The state is read once per page load and person, never polled; an
    // unread state is unknown (null), not "pending".
    assert.equal((control.match(/readGuideState\(\)/g) ?? []).length, 1);
    assert.match(control, /const decided = new Map/);
    const lib = read('../src/lib/guideState.ts');
    assert.match(lib, /readRemote\(GUIDE_STATE_URL/);
    assert.match(lib, /return read\.state === 'known' \? read\.value : null;/);
    assert.match(lib, /method: 'PUT'/);
    assert.match(lib, /JSON\.stringify\(\{ tour_version: version, tour_state: state \}\)/);
});

test('the tour and the drawer keep the keyboard, close on Escape and respect reduced motion', () => {
    const tourView = read('../src/help/guide/Tour.tsx');
    assert.match(tourView, /useFocusTrap\(dialog, onSkip, primary\)/);
    assert.match(tourView, /role="dialog"\s+aria-modal="true"/);
    assert.match(tourView, /t\('guide\.tour\.skip'\)/);
    assert.doesNotMatch(tourView.replace(/motion-safe:transition[-\w[\],]*/g, ''), /\btransition\b(?!-)/);
    const drawer = read('../src/help/guide/GuideDrawer.tsx');
    assert.match(drawer, /useFocusTrap\(panel, onClose, closeButton\)/);
    const trap = read('../src/help/guide/focusTrap.ts');
    assert.match(trap, /event\.key === 'Escape'/);
    assert.match(trap, /event\.key !== 'Tab'/);
    assert.match(trap, /before\.focus\(\)/);
});

test('the guide part is registered like every screen part and budgeted', () => {
    const index = read('../src/i18n/index.tsx');
    assert.match(index, /\(\) => import\('\.\/screens\/guide\/en'\)/);
    assert.match(index, /\(\) => import\('\.\/screens\/guide\/tr'\)/);
    assert.match(read('../src/i18n/en.ts'), /\| GuideScreenKey;/);
    const budget = read('../scripts/check-bundle-budget.mjs');
    assert.match(budget, /\n {2}guide: kib\(/);
    assert.match(budget, /`src\/help\/guide\/gen\/\$\{role\}\.\$\{locale\}\.ts`, guideChunkBudget/);
    assert.match(read('../package.json'), /"build": "node scripts\/build-guide\.mjs --check && tsc/);
});
