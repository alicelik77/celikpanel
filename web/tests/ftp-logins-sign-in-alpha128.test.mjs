// A site's other logins on a server whose SSH accepts only keys (alpha.128
// lab, Debian 13 and Ubuntu 24.04 cloud images). Logins sign in with a
// password, so SFTP is not possible there: with CelikPanel's FTP over TLS the
// list says logins connect with it (host, port) and SFTP is not offered;
// without it no login could sign in, so none can be added and the reason
// comes first. The administrator gets vsftpd's own install window and is told
// that allowing SSH passwords is their own decision (CelikPanel never changes
// sshd); anyone else is told to ask the provider. Said once, in one place.
// Yalnız anahtar kabul eden SSH'de ek girişler: şifreli FTP varsa onunla
// bağlanır; yoksa giriş eklenmez, neden ve kimin ne yapacağı önce söylenir.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const FolderLock = () => null;
  export const Plus = () => null;
  export const Trash2 = () => null;
  export const UserRound = () => null;
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => (path) => { globalThis.loginsNavigated.push(path); return true; };
  export const useAuth = () => ({ role: globalThis.loginsRole });
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const { FTPLogins } = await import(shared.compile('components/FTPLogins.tsx', {
  '/AdminInstallLink': compileSource('components/AdminInstallLink.tsx', () => stub),
}));

const view = ({ warnings, ftpInstalled, ftpConfig, logins = [] }) => ({
  domain: 'example.com', available: true, subscription_status: 'active',
  rights: { file_access: true }, owner: '', limit: { used: logins.length, ceiling: 10 },
  records: logins.map((login) => ({ login, folder: '/public_html' })),
  connection: { sftp_host: 'example.com', ftp_port: 21, sftp_port: 22, passive_min: 40000, passive_max: 40100 },
  state: {
    logins: logins.map((login) => ({ login, folder: '/public_html', account_ok: true, password: 'set', jail_active: true, unit_file: 'managed' })),
    server_logins: logins.length, sshd_present: true, sshd_drop_in: 'managed',
    ftp_installed: ftpInstalled, ftp_config: ftpConfig, warnings,
  },
});

const originalFetch = globalThis.fetch;
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let tree;
let calls;
async function mount(answer, role) {
  calls = [];
  globalThis.loginsNavigated = [];
  globalThis.loginsRole = role;
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = (url, options = {}) => {
    calls.push({ method: options.method || 'GET', url });
    return Promise.resolve(Response.json(answer));
  };
  await act(async () => {
    tree = Renderer.create(React.createElement(FTPLogins, { domainId: 5, isAdmin: role === 'admin' }));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const text = () => JSON.stringify(tree.toJSON());
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));
const buttonWith = (key) => tree.root.findAll((node) => node.type === 'button' && words(node).includes(key))[0];
const count = (needle) => text().split(needle).length - 1;

test('keys only and no FTP over TLS: the administrator sees why, installs vsftpd from its window, and adds nothing', async () => {
  await mount(view({ warnings: ['sftp_password_off'], ftpInstalled: false, ftpConfig: 'absent' }), 'admin');
  try {
    assert.ok(text().includes('ftpLogins.noSignIn.keys'), text());
    assert.ok(text().includes('ftpLogins.noSignIn.ftpMissingAdmin'));
    assert.ok(text().includes('ftpLogins.noSignIn.sshChoiceAdmin'), 'allowing SSH passwords is named as the owner’s own choice');
    assert.ok(!text().includes('ftpLogins.noSignIn.provider'));
    assert.equal(count('ftpLogins.warn.sftp_password_off'), 0, 'one notice, not two');
    assert.equal(buttonWith('ftpLogins.add'), undefined, 'a login that could not sign in is not offered');
    const link = tree.root.findAll((node) => node.type === 'button' && node.props['data-install-link'] === 'vsftpd')[0];
    assert.ok(link, 'the FTP server’s install window is one click away');
    await act(async () => link.props.onClick());
    assert.deepEqual(globalThis.loginsNavigated, ['/components?component=vsftpd&install=']);
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'nothing starts by itself');
  } finally {
    await cleanup();
  }
});

test('keys only and no FTP over TLS: a customer is told to ask the provider and gets no administrator action', async () => {
  await mount(view({ warnings: ['sftp_password_off'], ftpInstalled: false, ftpConfig: 'absent' }), 'customer');
  try {
    assert.ok(text().includes('ftpLogins.noSignIn.keys'));
    assert.ok(text().includes('ftpLogins.noSignIn.provider'));
    assert.ok(!text().includes('Admin'), text());
    assert.equal(tree.root.findAll((node) => node.type === 'button' && node.props['data-install-link']).length, 0);
    assert.equal(buttonWith('ftpLogins.add'), undefined);
  } finally {
    await cleanup();
  }
});

test('pam_shells (Arch) and FTP configured by hand: the reason is the system rule, and no SSH password advice', async () => {
  await mount(view({ warnings: ['sftp_shell_refused'], ftpInstalled: true, ftpConfig: 'owner' }), 'admin');
  try {
    assert.ok(text().includes('ftpLogins.noSignIn.shells'));
    assert.ok(text().includes('ftpLogins.noSignIn.ftpOwnerAdmin'));
    assert.ok(!text().includes('ftpLogins.noSignIn.sshChoiceAdmin'));
    assert.ok(!text().includes('ftpLogins.warn.sftp_shell_refused'), 'said once');
    assert.equal(buttonWith('ftpLogins.add'), undefined);
  } finally {
    await cleanup();
  }
});

test('keys only with CelikPanel’s FTP over TLS: logins connect with it (host, port), SFTP is not offered, adding works', async () => {
  await mount(view({ warnings: ['sftp_password_off'], ftpInstalled: true, ftpConfig: 'managed', logins: ['anna-up'] }), 'admin');
  try {
    assert.equal(count('ftpLogins.warn.sftp_password_off'), 1);
    assert.ok(text().includes('ftpLogins.warn.sftp_password_off{\\"host\\":\\"example.com\\",\\"port\\":21}'), text());
    assert.ok(!text().includes('ftpLogins.noSignIn'));
    assert.ok(!text().includes('ftpLogins.connectSftp'), 'SFTP is not offered where it cannot work');
    assert.ok(text().includes('ftpLogins.connectFtp'));
    assert.ok(buttonWith('ftpLogins.add'), 'a login can be added');
  } finally {
    await cleanup();
  }
});

test('password sign-in on: SFTP is offered and nothing is said about keys', async () => {
  await mount(view({ warnings: [], ftpInstalled: false, ftpConfig: 'absent', logins: ['anna-up'] }), 'admin');
  try {
    assert.ok(!text().includes('ftpLogins.noSignIn'));
    assert.ok(!text().includes('ftpLogins.warn.sftp_password_off'));
    assert.ok(text().includes('ftpLogins.connectSftp'));
    assert.ok(buttonWith('ftpLogins.add'));
  } finally {
    await cleanup();
  }
});
