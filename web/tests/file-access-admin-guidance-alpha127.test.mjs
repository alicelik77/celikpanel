// The Access tab, mounted (alpha.127; owner on Boston, 6 Nov 2026): the
// command line's missing tools and the missing FTP server are told to each
// reader with who acts. The administrator gets the action itself: "Install
// the tools" asks in the panel's own window, installs once with steps, reads
// the access again and the command-line option appears without a reload; FTP
// links to vsftpd's own install window. A customer is told that the provider
// acts and is offered no administrator action.
// Erişim sekmesi bağlanmış hâliyle: yönetici eylemi alır, müşteri sağlayıcısına sorar.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const Download = () => null;
  export const KeyRound = () => null;
  export const Plug = () => null;
  export const ShieldCheck = () => null;
  export const Trash2 = () => null;
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => (path) => { globalThis.accessNavigated.push(path); return true; };
  export const showToast = (...args) => globalThis.accessToasts.push(args);
`);
const shared = sharedLayer(stub);
const { DomainFileAccess } = await import(shared.compile('components/DomainFileAccess.tsx'));

const view = (jailMissing) => ({
  domain: 'example.com', available: true, subscription_status: 'active',
  rights: { file_access: true, ssh: true },
  connection: { ftp_host: 'example.com', sftp_host: 'example.com', server_ip: '192.0.2.1', ftp_port: 21, sftp_port: 22, username: 'example', start_dir: '/', passive_min: 40000, passive_max: 40100 },
  state: {
    account: { enabled: true, ssh: false, password: 'set', exists: true, home_matches: true, keys_file: 'panel', keys: [] },
    server: {
      sshd_present: true, ssh_drop_in: 'panel', password_authentication: 'yes', ftp_installed: false, ftp_config: '', ftp_certificate: '',
      fail2ban_installed: true, site_users_with_shell: 0,
      jail_missing: jailMissing, jail_packages: jailMissing, jail_unknown: [],
    },
  },
});

const originalFetch = globalThis.fetch;
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let tree;
let calls;
let installed;
function serve() {
  calls = [];
  installed = false;
  globalThis.accessNavigated = [];
  globalThis.accessToasts = [];
  globalThis.operationSteps = [];
  globalThis.operationEnds = [];
  globalThis.confirmAsked = [];
  globalThis.confirm = (text, request) => { globalThis.confirmAsked.push(request); return true; };
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, body: options.body ? JSON.parse(options.body) : undefined });
    if (method === 'POST' && url === '/api/v1/service/file-access-tools') {
      installed = true;
      return Promise.resolve(Response.json({ installed: ['unzip', 'zip'], still_missing: [] }));
    }
    return Promise.resolve(Response.json(view(installed ? [] : ['unzip', 'zip'])));
  };
}
async function mount(isAdmin) {
  serve();
  await act(async () => {
    tree = Renderer.create(React.createElement(DomainFileAccess, { domainId: 3, domainName: 'example.com', isAdmin }));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
  delete globalThis.confirm;
}
const text = () => JSON.stringify(tree.toJSON());
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));
const buttonWith = (key) => tree.root.findAll((node) => node.type === 'button' && words(node).includes(key))[0];

test('the administrator installs the tools in the panel window, once, and the option appears without a reload', async () => {
  await mount(true);
  try {
    assert.ok(text().includes('fileAccess.sshTools.missing'));
    assert.ok(text().includes('fileAccess.sshTools.admin'));
    assert.ok(!text().includes('fileAccess.sshTools.customer'), 'the administrator is not told to ask a provider');
    assert.ok(!text().includes('"fileAccess.ssh"'), 'the option waits for its tools');
    const install = buttonWith('fileAccess.sshTools.install');
    assert.ok(install, 'the install button is there');
    await act(async () => {
      install.props.onClick();
      for (let i = 0; i < 6; i++) await settled();
    });
    assert.equal(globalThis.confirmAsked.length, 1, 'asked once, in the panel window');
    assert.ok(globalThis.confirmAsked[0].title.startsWith('adminNext.access.confirmTitle'));
    const posts = calls.filter((call) => call.method === 'POST');
    assert.equal(posts.length, 1, 'installed once');
    assert.deepEqual(posts[0].body, { packages: ['unzip', 'zip'] });
    const last = globalThis.operationSteps.at(-1);
    assert.deepEqual(last.map((step) => [step.id, step.state]), [['install', 'done'], ['check', 'done']]);
    assert.equal(globalThis.operationEnds.at(-1).state, 'done');
    // The access was read again before the window said done: the option is back.
    assert.ok(!text().includes('fileAccess.sshTools.missing'), text());
    assert.ok(text().includes('fileAccess.ssh'));
  } finally {
    await cleanup();
  }
});

test('the administrator gets the FTP install link to vsftpd’s own install window', async () => {
  await mount(true);
  try {
    assert.ok(text().includes('fileAccess.block.ftpMissingAdmin'));
    assert.ok(!text().includes('"fileAccess.block.ftpMissing"'));
    const link = tree.root.findAll((node) => node.type === 'button' && node.props['data-install-link'] === 'vsftpd')[0];
    assert.ok(link, 'the install link is there');
    await act(async () => link.props.onClick());
    assert.deepEqual(globalThis.accessNavigated, ['/components?component=vsftpd&install=']);
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'the link starts nothing by itself');
  } finally {
    await cleanup();
  }
});

test('a customer is told the provider acts and is offered no administrator action', async () => {
  await mount(false);
  try {
    assert.ok(text().includes('fileAccess.sshTools.customer'));
    assert.ok(text().includes('fileAccess.block.ftpMissing'));
    assert.ok(!text().includes('fileAccess.block.ftpMissingAdmin'));
    assert.ok(!text().includes('fileAccess.sshTools.admin'));
    assert.equal(buttonWith('fileAccess.sshTools.install'), undefined);
    assert.equal(tree.root.findAll((node) => node.props?.['data-install-link']).length, 0);
  } finally {
    await cleanup();
  }
});

// alpha.127, last round: a plan or suspension notice opens the subscription
// account's rights or its resume window for whoever manages it; the account
// itself keeps the provider wording.
async function mountWith(body, isAdmin) {
  serve();
  globalThis.fetch = (url, options = {}) => {
    calls.push({ method: options.method || 'GET', url });
    return Promise.resolve(Response.json(body));
  };
  await act(async () => {
    tree = Renderer.create(React.createElement(DomainFileAccess, { domainId: 3, domainName: 'example.com', isAdmin }));
    await settled();
    await settled();
  });
}

test('whoever manages the subscription opens its rights or resumes it; the account itself is told its provider acts', async () => {
  const managed = { ...view([]), owner: 'aliuser', subscription_status: 'suspended', rights: { file_access: false, ssh: false } };
  await mountWith(managed, true);
  try {
    assert.ok(text().includes('adminNext.access.notInPlan'));
    assert.ok(text().includes('adminNext.access.suspended'));
    assert.ok(!text().includes('fileAccess.block.notInPlan') && !text().includes('fileAccess.block.suspended'));
    const links = tree.root.findAll((node) => node.type === 'button' && node.props['data-manager-link']);
    assert.deepEqual(links.map((node) => node.props['data-manager-link']), ['/users?rights=aliuser', '/users?resume=aliuser']);
    await act(async () => links[1].props.onClick());
    assert.deepEqual(globalThis.accessNavigated, ['/users?resume=aliuser']);
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
  } finally {
    await cleanup();
  }
  await mountWith({ ...managed, owner: '' }, false);
  try {
    assert.ok(text().includes('fileAccess.block.notInPlan') && text().includes('fileAccess.block.suspended'));
    assert.equal(tree.root.findAll((node) => node.type === 'button' && node.props['data-manager-link']).length, 0);
  } finally {
    await cleanup();
  }
});

test('a command line left out of the plan opens the account’s rights for whoever manages it', async () => {
  await mountWith({ ...view([]), owner: 'aliuser', rights: { file_access: true, ssh: false } }, true);
  try {
    assert.ok(text().includes('adminNext.access.sshNotInPlan'));
    const link = tree.root.findAll((node) => node.type === 'button' && node.props['data-manager-link'] === 'ssh')[0];
    await act(async () => link.props.onClick());
    assert.deepEqual(globalThis.accessNavigated, ['/users?rights=aliuser']);
  } finally {
    await cleanup();
  }
});
