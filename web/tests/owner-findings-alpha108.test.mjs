// alpha.108, four owner findings in the web interface, pinned as source
// contracts (layout, like the other layout contracts, is pinned by mechanism):
//   C7  the Components row keeps its name whole at medium widths;
//   C9  the left sidebar collapses to icons, by keyboard too, and is remembered;
//   C10 the Components row starts the SAME confirmed webmail update as Manage;
//   C11 owner terminal commands start with sudo, and the page says what root does.
//
// alpha.108, arayüzdeki dört sahip bulgusu.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';

const read = (relative) => readFileSync(new URL(relative, import.meta.url), 'utf8').replace(/\r\n/g, '\n');
const list = read('../src/components/ServiceList.tsx');
const layout = read('../src/components/Layout.tsx');
const flow = read('../src/components/RoundcubeUpdateFlow.tsx');
const card = read('../src/components/RoundcubeUpdateCard.tsx');
const ui = read('../src/components/ui.tsx');

// ---- C7 ----

function componentRow() {
  const start = list.indexOf('data-component-row=""');
  assert.ok(start > 0, 'the component row is no longer identifiable');
  const liStart = list.lastIndexOf('<li ', start);
  return { li: list.slice(liStart, start), body: list.slice(start, list.indexOf('</li>', start)) };
}

test('C7: the row is a grid only from 1280px; below that the name block keeps a minimum width and the actions wrap', () => {
  const { li, body } = componentRow();
  // The three columns need 432px (552px with the version column) beside the
  // 256px rail: at 768px that left the name about 50px.
  assert.match(li, /flex flex-wrap/);
  assert.match(li, /xl:grid /);
  assert.match(li, /xl:grid-cols-\[minmax\(0,1fr\)_7\.5rem_8rem_17rem\]/);
  assert.match(li, /xl:grid-cols-\[minmax\(0,1fr\)_8rem_17rem\]/);
  assert.doesNotMatch(li, /md:grid/, 'no grid at medium widths');
  // The name block cannot be squeezed below 15rem (or the row when it is narrower),
  // and takes the free room.
  assert.match(body, /min-w-\[min\(100%,15rem\)\] flex-1 basis-60/);
  assert.match(body, /xl:min-w-0/, 'inside the grid the cell is the column');
  // The status cell and the actions keep their own size and wrap as a unit.
  assert.match(body, /<div className="w-32 shrink-0 xl:w-auto">/);
  assert.match(body, /ml-auto flex min-w-0 flex-wrap/);
});

test('C7: a component name never breaks inside a hyphenated word', () => {
  const { body } = componentRow();
  assert.match(body, /<KeepWords text=\{componentName\(s, t\)\} \/>/);
});

test('C7: the phone line and the desktop version column switch at the same width as the grid', () => {
  const { body } = componentRow();
  assert.match(body, /className=\{versionColumn \? 'xl:hidden' : ''\}/);
  assert.match(body, /hidden min-w-0 xl:block" data-version-column=""/);
  assert.match(body, /basis-full xl:col-span-full/, 'an open version drawer spans the whole row');
});

// ---- C9 ----

test('C9: the sidebar toggle is a real button that says whether it is expanded and what it controls', () => {
  const toggle = layout.slice(layout.indexOf('function RailToggle'));
  assert.match(toggle, /<button\s+type="button"/);
  assert.match(toggle, /aria-expanded=\{!collapsed\}/);
  assert.match(toggle, /aria-controls="celikpanel-sidebar"/);
  assert.match(toggle, /aria-label=\{label\}/);
  assert.match(toggle, /title=\{label\}/);
  assert.match(layout, /<aside id="celikpanel-sidebar" className="hidden shrink-0 md:block"/);
  // The toggle is the first stop in the rail, before the items.
  const nav = layout.slice(layout.indexOf('<nav className'));
  assert.ok(nav.indexOf('<RailToggle') > 0 && nav.indexOf('<RailToggle') < nav.indexOf('<SidebarItem'));
});

test('C9: collapsed, every item keeps its name, a title and a tooltip on keyboard focus', () => {
  assert.match(layout, /\{\.\.\.\(rail \? \{ title: railName, 'aria-label': railName, \.\.\.tip\.handlers \} : \{\}\)\}/);
  const tip = layout.slice(layout.indexOf('function useRailTip'), layout.indexOf('function RailToggle'));
  assert.match(tip, /matches\(':focus-visible'\)/);
  assert.match(tip, /onBlur: \(\) => setAt\(null\)/);
  assert.match(tip, /event\.key === 'Escape'/);
  assert.match(tip, /position: 'fixed'/, 'the scrolling nav cannot clip it');
  assert.match(tip, /aria-hidden="true"/, 'the name is spoken once, from the button');
  // The count the badge would show is part of the collapsed name.
  assert.match(layout, /const railName = count === null/);
});

test('C9: the choice is remembered per browser, guarded, and the phone drawer is not collapsed', () => {
  const read = layout.slice(layout.indexOf('function readRailCollapsed'), layout.indexOf('export function Layout'));
  assert.equal((read.match(/try \{/g) ?? []).length, 2, 'both the read and the write are guarded');
  assert.match(read, /catch \{\s*return false;/, 'without storage the rail starts expanded');
  assert.match(layout, /useState\(readRailCollapsed\)/);
  assert.match(layout, /const content = buildContent\(false\);/, 'the phone drawer is built expanded and without the toggle');
  assert.match(layout, /\{desktop && <RailToggle/);
  assert.match(layout, /const rail = desktop && collapsed && !setupMode;/, 'the setup shell is never collapsed');
  assert.match(layout, /\$\{rail \? 'w-16' : 'w-64'\}/);
});

test('C9: two short shell strings, in English and Turkish', () => {
  for (const [name, words] of [['en', en], ['tr', tr]]) {
    assert.ok(words['nav.collapse'] && words['nav.expand'], name);
    assert.ok(words['nav.collapse'].length <= 24 && words['nav.expand'].length <= 24, name + ': short');
  }
  assert.notEqual(en['nav.collapse'], tr['nav.collapse']);
});

// ---- C10 ----

test('C10: the webmail update is started by one flow; the card and the Components row both use it', () => {
  assert.match(flow, /export function useRoundcubeUpdate/);
  assert.match(flow, /export function RoundcubeUpdateDialog/);
  assert.match(flow, /fetch\(ROUNDCUBE_UPDATE_URL/);
  assert.match(flow, /if \(starting\.current \|\| following\) return;/, 'a second start is refused while one runs');
  for (const [name, source] of [['card', card], ['row', list]]) {
    assert.match(source, /useRoundcubeUpdate\(/, name);
    assert.match(source, /<RoundcubeUpdateDialog /, name);
    assert.match(source, /<RoundcubeUpdateProgress /, name);
    assert.doesNotMatch(source.replace(/ROUNDCUBE_UPDATE_URL/g, ''), /fetch\([^)]*roundcube\/update/, name + ': no second implementation of the request');
  }
});

test('C10: the row button opens the confirmation, is disabled with a written reason while mutations are locked', () => {
  const row = list.slice(list.indexOf('function RoundcubeRowUpdate'), list.indexOf('function RowVersionText'));
  assert.match(row, /data-row-webmail-update-button=""/);
  assert.match(row, /onClick=\{\(\) => flow\.setConfirming\(true\)\}/, 'the button only opens the dialog');
  assert.match(row, /disabled=\{locked \|\| flow\.updating\}/);
  assert.match(row, /aria-describedby=\{locked \? 'webmail-row-update-locked' : undefined\}/);
  assert.match(row, /life\.webmail\.rowLocked/);
  assert.match(row, /relative/, 'it sits above the row link overlay');
  assert.match(list, /updateLocked=\{mutationControlsDisabled \|\| componentOperation\.locked\} updateLockedReason=\{readinessMessage\}/);
  // The result stays after the offer is gone (the re-read clears update_available).
  assert.match(row, /if \(!offered && !flow\.updating && !flow\.result && webmailUrl === ''\) return null;/);
  // The desktop version column shows the version alone: one button per row.
  assert.match(list, /props\.service\.id === 'roundcube' && props\.service\.is_installed && !props\.column/);
});

test('C10: the locked reason exists in both languages and says nothing needs doing', () => {
  assert.ok(enLifecycleScreens['life.webmail.rowLocked'].includes('Nothing needs to be done'));
  assert.ok(trLifecycleScreens['life.webmail.rowLocked'].includes('yapmanız gerekmez'));
});

// ---- C11 ----

function sourceFiles(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? sourceFiles(path) : /\.(ts|tsx)$/.test(name) ? [path] : [];
  });
}

test('C11: the note exists in both languages, as the owner worded it', () => {
  assert.equal(en['common.sudoNote'], 'If you are signed in as root, leave out sudo.');
  assert.equal(tr['common.sudoNote'], 'root olarak oturum açtıysanız sudo yazmayın.');
  assert.match(ui, /export function SudoNote/);
  assert.match(ui, /\{mentionsSudo\(errorText\) && <SudoNote \/>\}/, 'every banner that shows a sudo command carries it');
});

test('C11: the recovery page, the update outcome, the restarts list and the offline page carry the note beside their commands', () => {
  // The recovery screen's structure is read by the release-recovery harness
  // (deploy/e2e/release-recovery, parse_screen_source): the note is in the
  // sentence that introduces its one command, not a new element.
  assert.match(en['recovery.automatic.inspect'], /if you are signed in as root, leave out sudo/);
  assert.match(tr['recovery.automatic.inspect'], /root olarak oturum açtıysanız sudo yazmayın/);
  assert.doesNotMatch(read('../src/components/RecoveryAccess.tsx'), /data-sudo-note/);
  // The update screens use the key directly (their test doubles of ui.tsx carry no SudoNote).
  assert.match(read('../src/components/SystemUpdateOperation.tsx'), /failureGuidance\.command && \/\\bsudo\\b\/\.test\(failureGuidance\.command\) && <p[^>]*data-sudo-note/);
  assert.match(read('../src/components/PanelUpdateCard.tsx'), /data-sudo-note="">\{t\('common\.sudoNote'\)\}/);
  assert.match(read('../src/components/OperationOverlay.tsx'), /data-sudo-note="">\{t\('common\.sudoNote'\)\}/);
  assert.match(read('../src/components/RestartsCard.tsx'), /<SudoNote /);
  assert.match(read('../src/components/DomainSiteConfig.tsx'), /\{flag && <SudoNote \/>\}/);
  const offline = read('../src/offline/copy.ts');
  assert.ok(offline.includes('If you are signed in as root, leave out sudo.') && offline.includes('root olarak oturum açtıysanız sudo yazmayın.'));
});

test('C11: no catalogue or guidance text shows a root-only command without sudo', () => {
  const root = fileURLToPath(new URL('../src/', import.meta.url));
  const command = /(?<!sudo )(?<![\w./-])(systemctl (start|stop|restart|reload|status|enable|disable|daemon-reload)|journalctl -|phpenmod -|apt-get (install|update|remove|purge)|apt (install|update|remove)|dpkg-reconfigure|nginx -t\b(?! names)|postfix (check|reload|start))/;
  const found = [];
  for (const file of sourceFiles(root)) {
    // Comments are prose for developers; only strings the owner reads count.
    const text = readFileSync(file, 'utf8').split('\n').filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line)).join('\n');
    for (const match of text.matchAll(new RegExp(command, 'g'))) {
      const line = text.slice(0, match.index).split('\n').length;
      found.push(`${file.slice(root.length).replaceAll('\\', '/')}:${line}: ${match[0]}`);
    }
  }
  // The removal dialogue (ServiceList) names a command's family as a label, not a command line.
  assert.deepEqual(found.filter((entry) => !entry.startsWith('components/ServiceList.tsx')), []);
});
