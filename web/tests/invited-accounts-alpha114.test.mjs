import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

// Invited accounts (alpha.114): an invited account that has not chosen its
// password is never shown as active; its row offers Resend and Cancel; the
// team member editor can invite by email instead of setting a password; and a
// disabled Save always says what is missing. Every change re-reads every list
// that shows accounts, so no page refresh is needed.
// Davet edilen ve henüz giriş yapmayan hesap "etkin" görünmez; satırında yeniden
// gönder/iptal vardır; kapalı Kaydet neyin eksik olduğunu söyler.

const source = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const moduleURL = (text) => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');
const api = await import(moduleURL(ts.transpileModule(source('../src/lib/api.ts'), {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText));

const team = source('../src/components/TeamMembersPage.tsx');
const users = source('../src/components/UsersPage.tsx');
const invitations = source('../src/components/Invitations.tsx');
const en = source('../src/i18n/screens/accountAccess/en.ts');
const tr = source('../src/i18n/screens/accountAccess/tr.ts');

test('the invitation mark is read strictly from the list answer', () => {
  assert.deepEqual(api.parseAccountInvitationState({ id: 7, state: 'pending', expires_at: '2026-11-07T10:00:00Z' }),
    { id: 7, state: 'pending', expires_at: '2026-11-07T10:00:00Z' });
  assert.deepEqual(api.parseAccountInvitationState({ state: 'cancelled' }), { id: undefined, state: 'cancelled', expires_at: undefined });
  assert.equal(api.parseAccountInvitationState({ id: 7, state: 'active' }), undefined);
  assert.equal(api.parseAccountInvitationState(undefined), undefined);
  assert.equal(api.parseAccountInvitationState({ id: -1, state: 'expired' }).id, undefined);
});

test('invited accounts are not shown as active, in both lists', () => {
  for (const page of [users, team]) {
    assert.match(page, /status === 'active' && (u|member)\.invitation \?/);
    assert.match(page, /<InvitedStatus invitation=/);
  }
  assert.match(users, /cancelledHint=\{t\('acct\.invite\.status\.cancelledUsers'\)\}/);
  assert.match(team, /cancelledHint=\{t\('acct\.invite\.status\.cancelledTeam'\)\}/);
  assert.match(en, /'acct\.invite\.status\.pending': 'Invited — not signed in yet'/);
  assert.match(tr, /'acct\.invite\.status\.pending': 'Davet edildi — henüz giriş yapmadı'/);
});

test('resend and cancel act on the open invitation by id, once, and re-read every account list', () => {
  assert.match(invitations, /export function useInvitationActions\(\)/);
  assert.match(invitations, /if \(busy !== null\) return;/);
  assert.match(invitations, /fetch\(`\$\{INVITATIONS_URL\}\/\$\{id\}`, \{ method: 'DELETE' \}\)/);
  assert.match(invitations, /postJSON\(`\$\{INVITATIONS_URL\}\/\$\{id\}\/resend`, \{ delivery: action, lang: locale \}\)/);
  assert.match(invitations, /invalidate\('users', 'team'\)/);
  // Only a row with an open link offers them; a cancelled one has no id.
  assert.match(users, /u\.invitation\?\.id !== undefined/);
  assert.match(team, /member\.invitation\?\.id !== undefined/);
  assert.match(users, /act\(id, username, 'email'\)/);
  assert.match(users, /act\(id, username, 'revoke'\)/);
  assert.match(team, /onInvitation\(member, 'email'\)/);
  assert.match(team, /onInvitation\(member, 'revoke'\)/);
});

test('the team member editor can invite by email, and shows what happened', () => {
  assert.match(team, /data-invite-option=''/);
  assert.match(team, /password: draft\.invite \? '' : draft\.password/);
  assert.match(team, /\.\.\.\(draft\.invite \? \{ invite: true, lang: locale \} : \{\}\)/);
  assert.match(team, /if \(draft\.invite\) invitation\.report\(created\.invitation\)/);
  assert.match(team, /\{invitation\.dialog\}/);
  assert.match(source('../src/lib/api.ts'), /return \{ member: parseTeamMember\(payload\.team_member\), invitation: payload\.invitation \};/);
});

test('a disabled Save says what is missing', () => {
  assert.match(team, /id='team-save-reason'/);
  assert.match(team, /t\('acct\.team\.missing', \{ fields: missing\.join\(', '\) \}\)/);
  assert.match(team, /newPasswordShort \? \[t\('acct\.team\.newPasswordShort'\)\]/);
  assert.match(team, /aria-describedby=\{!canSave \? 'team-save-reason' : undefined\}/);
  assert.match(users, /id="plan-save-reason"/);
  for (const key of ['acct.team.missing', 'acct.team.passwordRule', 'acct.team.newPasswordShort', 'acct.plan.missing',
    'acct.team.inviteHint', 'acct.invite.rowResend', 'acct.invite.status.cancelledUsers', 'acct.invite.status.cancelledTeam']) {
    assert.ok(en.includes(`'${key}':`), `${key} in English`);
    assert.ok(tr.includes(`'${key}':`), `${key} in Turkish`);
  }
});

test('the team list re-reads on any change, and the re-read only reads', () => {
  assert.match(team, /useInvalidation\(\['team'\], \(\) => void load\(true\)\)/);
  assert.match(team, /const changed = \(\) => invalidate\('team', 'users'\);/);
  assert.equal((team.match(/changed\(\);/g) ?? []).length, 3, 'save, status and delete each re-read');
  const load = team.slice(team.indexOf('const load = async'), team.indexOf('useEffect(() => {'));
  assert.doesNotMatch(load, /create|update|delete/i);
  assert.match(users, /invalidate\('users', 'plans'\)/);
});
