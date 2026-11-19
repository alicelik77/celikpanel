// alpha.140: the setup review's zone-wide rule (unchanged) refuses a main
// domain whose zone has another hosted zone inside it or around it. The plan
// review and the peer join now say that cause in its own words: not a record
// clash, not the second server's names. The words of both, in both languages,
// and the codes that select them.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { setupExecutionGuidance } from '../src/lib/serverSetupGuidance.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const context = { dns_mode: 'local', dns_role: 'primary', dns_engine: 'pdns', local_nameserver: 'ns1.example.com', local_ip: '192.0.2.10', peer_nameserver: 'ns2.example.com', peer_ip: '192.0.2.20', panel_domain: 'panel.example.com', mail_hostname: '', dns_hosting_management: 'panel' };
const waitingFor = (code) => ({ status: 'waiting', phase: 'peer_join', steps: [{ id: '02-peer_join', kind: 'peer_join', target: 'second-server', status: 'running' }], error: { code, message: 'x' }, context });
const keys = (guide) => [...guide.messages, ...guide.details].map((item) => item.key);

test('the peer join waiting for an overlapping zone says that cause, who acts and what next, in both languages', () => {
    const guide = setupExecutionGuidance(waitingFor('server_setup_peer_zone_overlap'));
    assert.ok(keys(guide).includes('setup.v5.wait.server_setup_peer_zone_overlap'));
    assert.ok(!keys(guide).includes('setup.v5.wait.server_setup_peer_names_clash'));
    const en = enScreens['setup.v5.wait.server_setup_peer_zone_overlap'];
    const tr = trScreens['setup.v5.wait.server_setup_peer_zone_overlap'];
    assert.match(en, /another DNS zone hosted on this server lies inside it or around it/);
    assert.match(en, /This is not about that server’s names\. Who acts: this server’s administrator\. Next: review that zone in Domains/);
    assert.doesNotMatch(en, /Change them/);
    assert.match(tr, /başka bir DNS bölgesi onun içinde ya da çevresinde/);
    assert.match(tr, /Kim işlem yapar: bu sunucunun yöneticisi\. Sonraki adım: o bölgeyi Alan adları’nda gözden geçirin/);
    // Control: a real clash of the second server's names keeps its own sentence.
    assert.ok(keys(setupExecutionGuidance(waitingFor('server_setup_peer_names_clash'))).includes('setup.v5.wait.server_setup_peer_names_clash'));
});

test('the plan review blocker for an overlapping zone is its own problem, not a record clash', () => {
    const what = enScreens['setup.problem.infraOverlap.what'];
    const todo = enScreens['setup.problem.infraOverlap.todo'];
    assert.match(what, /inside the main domain’s zone or around it/);
    assert.match(what, /This is not a clash with a record or with a name you chose/);
    assert.doesNotMatch(what + todo, /Skip the clashing record|give the server another name/);
    assert.match(todo, /administrator reviews that zone in Domains; then read the plan again/);
    assert.match(trScreens['setup.problem.infraOverlap.what'], /bir kayıtla ya da seçtiğiniz bir adla çakışma değil/);
    assert.match(trScreens['setup.problem.infraOverlap.todo'], /yöneticisi o bölgeyi Alan adları’nda gözden geçirir/);
    // The codes: the Panel sends the blocker and the wait, the screen maps them.
    assert.match(read('../src/lib/setupProblems.ts'), /server_setup_infrastructure_dns_zone_overlap: \{ name: 'infraOverlap', fix: recheck \}/);
    assert.match(read('../../cmd/panel/server_setup_operations.go'), /code = "infrastructure_dns_zone_overlap"/);
    assert.match(read('../../cmd/panel/server_setup_peer_join.go'), /Code: "server_setup_peer_zone_overlap"/);
    assert.match(read('../src/components/ServerSetup.tsx'), /server_setup_peer_zone_overlap: 'setup\.v5\.wait\.server_setup_peer_zone_overlap'/);
});
