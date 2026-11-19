#!/usr/bin/env bash
# Execute early idle admission with a private probe; no installed paths/services.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf -- "$TEST_ROOT"' EXIT
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
extract() { sed -n "/^$2() {$/,/^}$/p" "$ROOT/$1"; }
export TEST_ROOT
cat > "$TEST_ROOT/probe" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[[ $# == 1 && $1 == --check-service-mutation-idle ]] || exit 90
[[ $CELIKPANEL_AGENT_STATE_DIR == "$TEST_ROOT/state" && $CELIKPANEL_MUTATION_LOCK == "$TEST_ROOT/lock" ]] || exit 91
printf 'probe\n' >> "$TEST_ROOT/calls"
case $PROBE_STATE in
 idle) exit 0 ;;
 enrollment) echo 'privileged mutation aabb is active' >&2; exit 1 ;;
 finishing)
  # The Agent finishes its running operation on the third read.
  [[ $(wc -l < "$TEST_ROOT/calls") -ge 3 ]] && exit 0
  echo 'service mutation state is not idle: privileged mutation ccdd is active' >&2; exit 1 ;;
 package)
  # Debian/Ubuntu's daily apt run ends on the third read (alpha.136 lab).
  [[ $(wc -l < "$TEST_ROOT/calls") -ge 3 ]] && exit 0
  echo 'Service mutation idle check: service mutation state is not idle: the host package manager is active' >&2; exit 1 ;;
 package-lasting) echo 'Service mutation idle check: service mutation state is not idle: the host package manager is active' >&2; exit 1 ;;
 unknown) echo 'mutation evidence changed' >&2; exit 2 ;;
 *) exit 92 ;;
esac
SH
chmod 0755 "$TEST_ROOT/probe"
eval "$(extract update.sh run_update_idle_probe)"
eval "$(extract update.sh update_agent_refusal_is_transient)"
eval "$(extract update.sh update_refusal_is_package_manager)"
eval "$(extract update.sh run_update_probe_waiting)"
eval "$(extract update.sh run_update_agent_idle_probe_waiting)"
eval "$(grep -E '^UPDATE_AGENT_IDLE_(WAIT|POLL)_SECONDS=' "$ROOT/update.sh")"
[[ $UPDATE_AGENT_IDLE_WAIT_SECONDS == 60 && $UPDATE_AGENT_IDLE_POLL_SECONDS == 3 ]] \
 || fail 'bounded Agent idle wait is not 60 s polled every 3 s'
# The wait is bounded by the poll count; no real time passes in this test.
sleep() { printf 'sleep %s\n' "$*" >> "$TEST_ROOT/sleeps"; }
eval "$(extract update.sh preflight_mutations_before_quiesce)"
# upd4: the refusal is reported typed through the updater's own helper.
eval "$(extract update.sh update_probe_diagnostic)"
eval "$(extract update.sh fail_update_preflight)"
die() { printf 'refused: %s; detail=%s\n' "$*" "$update_failure_detail" >&2; exit 73; }
# The checks below keep die: only the first check reports its own stop.
PREFLIGHT_AGENT=$TEST_ROOT/probe
AGENT_STATE_DIR=$TEST_ROOT/state
MUTATION_LOCK=$TEST_ROOT/lock
for PROBE_STATE in idle enrollment unknown finishing package package-lasting; do
 export PROBE_STATE
 BOOTSTRAP_PRE_LEDGER=0
 : > "$TEST_ROOT/calls"
 : > "$TEST_ROOT/sleeps"
 status=0
 (preflight_mutations_before_quiesce; printf 'reached-quiesce\n') > "$TEST_ROOT/output" 2>&1 || status=$?
 calls=$(wc -l < "$TEST_ROOT/calls")
 case $PROBE_STATE in
 # A running Agent operation is waited for, bounded: one read plus one per 3 s poll.
 enrollment) [[ $calls == 21 && $(wc -l < "$TEST_ROOT/sleeps") == 20 ]] || fail "running operation was not waited for exactly 60 s ($calls reads)"
  grep -q 'The Agent is finishing another server operation; waiting up to 60 s' "$TEST_ROOT/output" || fail 'the wait was not announced' ;;
 # The host package manager is waited for with the same bound, and the line
 # says so (never "the Agent is finishing an operation").
 package-lasting) [[ $calls == 21 && $(wc -l < "$TEST_ROOT/sleeps") == 20 ]] || fail "package manager was not waited for exactly 60 s ($calls reads)"
  grep -q "The server's package manager is working (for example automatic updates); waiting up to 60 s for it before the update continues (nothing needs to be done)" "$TEST_ROOT/output" || fail 'the package-manager wait was not announced'
  grep -q 'Sunucunun paket yöneticisi çalışıyor (örneğin otomatik güncellemeler); güncelleme devam etmeden önce en çok 60 sn bekleniyor (bir şey yapmanız gerekmiyor)' "$TEST_ROOT/output" || fail 'the Turkish package-manager wait line is missing'
  ! grep -q 'The Agent is finishing' "$TEST_ROOT/output" || fail 'the package-manager wait named the Agent' ;;
 package) [[ $calls == 3 ]] || fail "finishing package manager was read $calls times" ;;
 finishing) [[ $calls == 3 ]] || fail "finishing operation was read $calls times" ;;
 # Unknown or unsafe evidence is never waited for.
 *) [[ $calls == 1 && ! -s "$TEST_ROOT/sleeps" ]] || fail 'probe was not called exactly once' ;;
 esac
 case $PROBE_STATE in
 idle) [[ $status == 0 ]] && grep -q 'reached-quiesce' "$TEST_ROOT/output" || fail 'idle was not admitted' ;;
 finishing) [[ $status == 0 ]] && grep -q 'reached-quiesce' "$TEST_ROOT/output" || fail 'an operation that finished within the wait was refused'
  grep -q 'finished its operation after about 6 s' "$TEST_ROOT/output" || fail 'the end of the wait was not announced' ;;
 package) [[ $status == 0 ]] && grep -q 'reached-quiesce' "$TEST_ROOT/output" || fail 'a package run that finished within the wait was refused'
  grep -q 'The package manager finished after about 6 s; the update continues' "$TEST_ROOT/output" || fail 'the end of the package-manager wait was not announced'
  grep -q 'Paket yöneticisi işini yaklaşık 6 sn sonra bitirdi; güncelleme sürüyor' "$TEST_ROOT/output" || fail 'the Turkish end of the package-manager wait is missing'
  ! grep -q 'The Agent' "$TEST_ROOT/output" || fail 'the package-manager wait named the Agent' ;;
 package-lasting) [[ $status == 73 ]] && ! grep -q 'reached-quiesce' "$TEST_ROOT/output" || fail 'a lasting package run reached quiesce'
  grep -q ': the host package manager is active' "$TEST_ROOT/output" || fail 'the package-manager refusal lost its sentence'
  grep -q 'coordinators remain available' "$TEST_ROOT/output" || fail 'refusal lost recovery guidance' ;;
 *) [[ $status == 73 ]] && ! grep -q 'reached-quiesce' "$TEST_ROOT/output" || fail 'non-idle reached quiesce'
    grep -q 'coordinators remain available' "$TEST_ROOT/output" || fail 'refusal lost recovery guidance'
    if [[ $PROBE_STATE == enrollment ]]; then grep -q 'privileged mutation aabb is active' "$TEST_ROOT/output" || fail 'reservation detail lost'
    else grep -q 'mutation evidence changed' "$TEST_ROOT/output" || fail 'unknown became idle'; fi ;;
 esac
done
# Historical pre-ledger bootstrap retains its existing, separately strict probes.
BOOTSTRAP_PRE_LEDGER=1
: > "$TEST_ROOT/calls"
preflight_mutations_before_quiesce
[[ ! -s "$TEST_ROOT/calls" ]] || fail 'pre-ledger compatibility changed'
# alpha.137 lab: the first check, before the recovery runtime's preparation and
# before the EXIT trap, waits the same bound; a package manager still busy
# after it ends as the typed unchanged stop that update.sh writes itself
# (the Agent then clears the attempt's unit; the panel offers the update again).
eval "$(extract update.sh preflight_host_idle_before_recovery_runtime)"
eval "$(extract update.sh report_update_failure)"
eval "$(extract update.sh publish_update_failure_observation)"
for PROBE_STATE in package package-lasting enrollment; do
 export PROBE_STATE
 BOOTSTRAP_PRE_LEDGER=0
 : > "$TEST_ROOT/calls"
 : > "$TEST_ROOT/sleeps"
 status=0
 (preflight_host_idle_before_recovery_runtime; printf 'reached-preparation\n') > "$TEST_ROOT/output" 2>&1 || status=$?
 last=$(tail -n 1 "$TEST_ROOT/output")
 case $PROBE_STATE in
 package) [[ $status == 0 ]] && grep -q 'reached-preparation' "$TEST_ROOT/output" || fail 'a package run that ended within the first wait was refused'
  grep -q 'The package manager finished after about 6 s; the update continues' "$TEST_ROOT/output" || fail 'the first wait was not announced' ;;
 package-lasting) [[ $status == 1 && $(wc -l < "$TEST_ROOT/sleeps") == 20 ]] || fail "a lasting package run before preparation: status $status"
  ! grep -q 'reached-preparation' "$TEST_ROOT/output" || fail 'a lasting package run reached the preparation'
  [[ $last == '!! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=the host package manager is active detail=' ]] ||
   fail "a lasting package run before preparation lost its typed unchanged stop: $last" ;;
 enrollment) [[ $status == 1 ]] || fail "a lasting operation before preparation: status $status"
  [[ $last == '!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=agent_idle class=check_failed: '* ]] ||
   fail "a lasting operation before preparation lost its typed unchanged stop: $last" ;;
 esac
done
BOOTSTRAP_PRE_LEDGER=1
: > "$TEST_ROOT/calls"
preflight_host_idle_before_recovery_runtime
[[ ! -s "$TEST_ROOT/calls" ]] || fail 'pre-ledger bootstrap gained a first wait'
python3 - "$ROOT" <<'PY'
from pathlib import Path
import sys
s=(Path(sys.argv[1])/'update.sh').read_text()
first=s.index('    preflight_host_idle_before_recovery_runtime\n')
prepare=s.index('    prepare_independent_recovery_runtime\n',first)
reloads=s.index('    release_txn_install_and_verify_unit_guards',prepare)
assert first<prepare<reloads, 'the first wait does not precede the recovery runtime preparation'
# Only the first check reports its own unchanged stop (fail_update_preflight
# reads a flag no other caller sets); it is called once, before the release
# recovery intent and the EXIT trap, and after the preflight binaries are
# validated.
flag='local update_preflight_reports_unchanged_stop=1\n'
assert s.count('update_preflight_reports_unchanged_stop=1')==1, 'the unchanged-stop flag is set elsewhere'
function=s.index('preflight_host_idle_before_recovery_runtime() {\n')
assert function<s.index(flag)<s.index('\n}\n',function), 'the flag is not local to the first check'
assert s.count('\n    preflight_host_idle_before_recovery_runtime\n')==1
assert first<s.index('\n    publish_release_recovery_intent\n')<s.index('\ntrap on_exit EXIT\n')
assert s.index('\nvalidate_preflight_binary "$PREFLIGHT_AGENT" agent\n')<first, 'the first check runs an unvalidated preflight binary'
early=s.index('    preflight_mutations_before_quiesce\n')
stage=s.index('    mkdir -m 0700 -- "$stage_root"',early)
marker=s.index('    release_txn_create_quiesce_marker',stage)
locked=s.index('--check-service-mutation-idle-under-external-lock',marker)
freeze=s.index('    freeze_release_service_cgroup celikpanel-panel.service',locked)
assert early<stage<marker<locked<freeze
PY
printf 'PASS: a running Agent operation or the host package manager is waited for at most 60 s, before the recovery runtime preparation (typed unchanged stop) and again before the BIND preflight; pending or unknown mutation refuses before update barriers; final locked proof retained\n'
