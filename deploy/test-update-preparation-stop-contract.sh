#!/usr/bin/env bash
# The updater's untyped "recovery runtime preparation is unconfirmed" stop is
# one free-text line read in three places: update.sh writes it, the Agent
# clears the attempt's unit by it (cmd/agent system_update_store_linux.go) and
# the update window shows a closable stop by it (web/src/lib/
# systemUpdateFailure.ts). The recovery tool's typed busy reports are Go text
# that the recovery runtime shell contract copies as fixtures. This contract
# keeps them one text: the die line occurs once, both readers match it exactly
# as the Agent records it, and nothing but a stop before any change can say it.
# Hazırlığı doğrulanamayan duruş satırı üç yerde okunur; bu sözleşme onu tek
# metin olarak tutar.
set -euo pipefail

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

python3 - "$ROOT" <<'PY'
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
update = (root / 'update.sh').read_text(encoding='utf-8')
agent = (root / 'cmd/agent/system_update_store_linux.go').read_text(encoding='utf-8')
web = (root / 'web/src/lib/systemUpdateFailure.ts').read_text(encoding='utf-8')
entry = (root / 'cmd/recovery/promotion_entry.go').read_text(encoding='utf-8')
manifest = (root / 'internal/recoveryruntime/manifest.go').read_text(encoding='utf-8')
shell = (root / 'deploy/test-recovery-runtime-shell-contract.sh').read_text(encoding='utf-8')


def fail(message):
    sys.exit('FAIL: ' + message)


# 1. The die line: once in update.sh, in prepare_independent_recovery_runtime.
dies = re.findall(r'die "(independent recovery runtime preparation is unconfirmed[^"]*)"', update)
if len(dies) != 1:
    fail(f'update.sh has {len(dies)} "preparation is unconfirmed" die lines, want 1')
line = dies[0]
if update.count('independent recovery runtime preparation is unconfirmed') != 1:
    fail('the "preparation is unconfirmed" text appears outside its one die line')
function = update.index('\nprepare_independent_recovery_runtime() {\n')
end = update.index('\n}\n', function)
if not function < update.index(line) < end:
    fail('the die line is not inside prepare_independent_recovery_runtime')

# 2. Its one caller precedes the release-recovery intent and the EXIT trap:
#    the stop comes before any release transaction or service stop.
calls = [m.start() for m in re.finditer(r'\n    prepare_independent_recovery_runtime\n', update)]
if len(calls) != 1:
    fail(f'prepare_independent_recovery_runtime is called {len(calls)} times, want 1')
intent = update.index('\n    publish_release_recovery_intent\n')
trap = update.index('\ntrap on_exit EXIT\n')
if not calls[0] < intent < trap:
    fail('the preparation is not called before the release-recovery intent and the EXIT trap')

# 3. Both readers match the line as the Agent records it, and only it.
agent_re = re.search(r'systemUpdatePreparationUnconfirmedLineRE = regexp\.MustCompile\(`([^`]+)`\)', agent)
web_re = re.search(r'systemUpdateRecoveryPreparationUnconfirmed\(diagnostic: string\): boolean \{\n\s*return /(.+)/\.test\(diagnostic\);', web)
if not agent_re or not web_re:
    fail('the Agent or the web reader of the line was not found')
recorded = 'reviewed updater failed: exit status 1: !! ' + line
others = [
    'reviewed updater failed: exit status 1: !! independent recovery runtime preparation is unconfirmed after the services stopped',
    'reviewed updater failed: exit status 1: !! independent firewall preparation is unconfirmed; panel services have not been stopped; preserve its files',
    'reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=x detail=',
]
for name, pattern in (('Agent', agent_re.group(1)), ('web', web_re.group(1))):
    compiled = re.compile(pattern)
    if not compiled.search(recorded):
        fail(f'the {name} reader does not match the die line as recorded: {recorded}')
    for other in others:
        if compiled.search(other):
            fail(f'the {name} reader matches another line: {other}')

# 4. The recovery tool's typed busy reports, rebuilt from the Go source, are
#    exactly the fixture lines the shell contract feeds update.sh.
codes = dict(re.findall(r'\n\t(Reason\w+)\s+Reason = "([a-z_]+)"', manifest))
reports = re.findall(r'report\("([^"]*)" \+ string\(recoveryruntime\.(Reason\w+)\) \+ "([^"]*)"\)', entry)
if len(reports) != 2:
    fail(f'cmd/recovery has {len(reports)} typed busy reports, want 2')
for before, reason, after in reports:
    text = before + codes[reason] + after
    if shell.count("sys.stderr.write('" + text + "\\n')") != 1:
        fail(f'the shell contract does not feed the recovery tool\'s exact report: {text}')
    if '(' + codes[reason] + ')' not in update:
        fail(f'update.sh does not match the code {codes[reason]}')

print('PASS: the unconfirmed-preparation stop is one line, read alike by the Agent and the window, said only before the release-recovery intent and the EXIT trap; the typed busy reports are the recovery tool\'s own text')
PY
