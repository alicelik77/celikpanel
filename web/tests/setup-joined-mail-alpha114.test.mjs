import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

// alpha.114 (owner, 2026-10-31): a server joined by code runs mail for its
// own customers' domains. Its mail name is proposed under its own panel name,
// a name the first server already uses is refused, and the Names step and
// review say that the main domain's records stay on the first server.
// Koda katilan sunucu kendi posta adini alir; ana alan adi ilk sunucuda kalir.

const dataURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText;
const setup = await import(dataURL(compile('../src/lib/serverSetup.ts')));
const source = path => readFileSync(new URL(path, import.meta.url), 'utf8');

const joined = { firstMail: 'mail.celikhost.com', firstPanel: 'frankfurt.celikhost.com', mainDomain: 'celikhost.com', ns1: 'ns1.celikhost.com', ns2: 'ns2.celikhost.com' };

test('a joined server proposes its own mail name under its panel name', () => {
    const general = setup.setupProposedNames('celikhost.com', 'boston');
    assert.equal(general.mail, 'mail.celikhost.com');
    assert.equal(setup.setupJoinedMailProposal(general, 'boston.celikhost.com').mail, 'mail.boston.celikhost.com');
    assert.equal(setup.setupJoinedMailProposal(general, 'Boston.Celikhost.com.').mail, 'mail.boston.celikhost.com');
    // An unfinished panel name falls back to the proposed one.
    assert.equal(setup.setupJoinedMailProposal(general, 'boston').mail, 'mail.boston.celikhost.com');
});

test('the first server\'s names, the main domain and the nameservers are taken; the panel name is not', () => {
    for (const name of ['mail.celikhost.com', 'MAIL.celikhost.com.', 'frankfurt.celikhost.com', 'celikhost.com', 'ns1.celikhost.com', 'ns2.celikhost.com']) {
        assert.equal(setup.setupJoinedMailTaken(name, joined), true, name);
    }
    for (const name of ['mail.boston.celikhost.com', 'boston.celikhost.com', 'mail2.celikhost.com', '']) {
        assert.equal(setup.setupJoinedMailTaken(name, joined), false, name);
    }
});

test('the generic earlier proposal is replaced, a name the owner typed is kept', () => {
    const proposal = { panel: 'boston.celikhost.com', mail: 'mail.boston.celikhost.com', ns1: '', ns2: '' };
    const draft = mail => ({ panel_domain: 'boston.celikhost.com', mail_hostname: mail });
    assert.equal(setup.setupApplyJoinedMailName(draft('mail.celikhost.com'), proposal, joined).mail_hostname, 'mail.boston.celikhost.com');
    assert.equal(setup.setupApplyJoinedMailName(draft('boston.celikhost.com'), proposal, joined).mail_hostname, 'boston.celikhost.com');
    assert.equal(setup.setupApplyJoinedMailName(draft('frankfurt.celikhost.com'), proposal, joined).mail_hostname, 'frankfurt.celikhost.com');
    // When the first server has no mail, mail.<main domain> is free.
    assert.equal(setup.setupApplyJoinedMailName(draft('mail.celikhost.com'), proposal, { ...joined, firstMail: '' }).mail_hostname, 'mail.celikhost.com');
});

test('joined-server texts exist in English and Turkish and are used on the Names step and review', () => {
    const en = source('../src/i18n/screens/setupWizard/en.ts');
    const tr = source('../src/i18n/screens/setupWizard/tr.ts');
    const keys = ['setup.v6.names.joinedNote', 'setup.v6.names.mailJoinedHelp', 'setup.v6.names.mailTaken', 'setup.v6.names.mailTakenShort',
        'setup.v6.review.joinedSite', 'setup.v6.review.joinedMail', 'setup.v6.blocker.mailHostnameTaken'];
    for (const key of keys) {
        assert.ok(en.includes(`"${key}"`), `en ${key}`);
        assert.ok(tr.includes(`"${key}"`), `tr ${key}`);
    }
    // The main domain's mail is said to stay on the first server; no claim of MX/SPF writes here.
    assert.match(en, /"setup\.v6\.review\.joinedMail": "Mail for \{domain\} stays on \{first\}; its MX, SPF, DKIM and DMARC do not change/);
    // The blocker says who acts and what to do (D-024).
    assert.match(en, /"setup\.v6\.blocker\.mailHostnameTaken": "[^"]*You, the owner: go back to This server’s names/);
    assert.match(tr, /"setup\.v6\.blocker\.mailHostnameTaken": "[^"]*Siz, sunucu sahibi:/);
    const wizard = source('../src/components/ServerSetup.tsx');
    const steps = source('../src/components/ServerSetupWizardSteps.tsx');
    assert.match(wizard, /server_setup_peer_mail_hostname_taken: 'setup\.v6\.blocker\.mailHostnameTaken'/);
    // A joined server never shows the "import them in Existing records" line or the mailNow claim when the first keeps the mail.
    assert.match(wizard, /roles\.web && mainDomain && !joinedLink && !\(localPrimary/);
    assert.match(wizard, /mailSelected && mainDomain && !firstKeepsMainMail && <li>\{t\(draft\.mail_dns/);
    assert.match(wizard, /joinedLink && <li>\{t\('setup\.v6\.review\.joinedSite'/);
    assert.match(steps, /joined \? t\('setup\.v6\.names\.joinedNote'/);
});
