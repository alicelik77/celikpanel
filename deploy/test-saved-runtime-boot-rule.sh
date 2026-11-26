#!/usr/bin/env bash
# alpha.150 (alpha.149 lab, step 33 B): a server restart inside an update's or
# a return's pending window. celikpanel-firewall-restore.service is a boot
# oneshot; saved inactive, the boot runs it, and every finalization attempt
# refused "saved inactive-like service became active". The three scripts that
# verify the saved runtime states (update.sh, the pending-update and the
# pending-rollback finalizers) carry one identical rule: the change is accepted
# only for that unit, only after a boot recorded with the saved states, and only
# when the boot's own successful run made it active. Everything else refuses as
# before. The real functions run against a systemctl stand-in and a boot id file.
# Bekleyen pencerede yeniden başlatma: üç betik aynı kuralı taşır ve gerçek
# işlevler sahte systemctl ile çalıştırılır.
set -euo pipefail
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT

scripts=(update.sh deploy/finalize-pending-update.sh deploy/finalize-pending-rollback.sh)
functions=(load_saved_service_states_boot firewall_restore_activated_by_boot verify_saved_runtime_states)
extract() {
    awk -v header="$2() {" '$0 == header { inside=1 } inside { print } inside && $0 == "}" { exit }' "$1"
}

# Static: one rule, byte for byte, in every script that verifies saved states.
for fn in "${functions[@]}"; do
    reference=$(extract "$repo_root/${scripts[0]}" "$fn")
    [[ -n $reference ]] || fail "${scripts[0]} lacks $fn"
    for script in "${scripts[@]:1}"; do
        [[ $(extract "$repo_root/$script" "$fn") == "$reference" ]] ||
            fail "$script: $fn differs from update.sh's"
    done
done
for script in "${scripts[@]}"; do
    file=$repo_root/$script
    grep -Fxq 'KERNEL_BOOT_ID_FILE=/proc/sys/kernel/random/boot_id' "$file" || fail "$script: boot id path is not fixed"
    grep -Fxq 'FIREWALL_POLICY_FILE=/etc/celikpanel/firewall.nft' "$file" || fail "$script: firewall policy path is not fixed"
    grep -Fxq 'SERVICE_STATES_BOOT_RECORD=service-states.boot-id' "$file" || fail "$script: boot record name is not fixed"
    # The loader reads the boot record as its last step, for every ledger it loads.
    loader=$(extract "$file" load_saved_service_states)
    [[ $(printf '%s\n' "$loader" | tail -n 2 | head -n 1) == '    load_saved_service_states_boot "$ledger"' ]] ||
        fail "$script: load_saved_service_states does not end by loading the boot record"
done
# update.sh writes the record beside the ledger it has just written, in the
# same boot, before the ledger is loaded and made durable.
update=$repo_root/update.sh
write_line=$(grep -nF 'printf '"'"'%s\n'"'"' "$saved_states_boot" > "$tmp_snap/$SERVICE_STATES_BOOT_RECORD"' "$update" | cut -d: -f1)
ledger_line=$(grep -nF '>> "$tmp_snap/service-states.tsv"' "$update" | head -n 1 | cut -d: -f1)
load_line=$(awk -v start="${write_line:-0}" 'NR > start && index($0, "load_saved_service_states \"$tmp_snap/service-states.tsv\"") { print NR; exit }' "$update")
sync_line=$(awk -v start="${write_line:-0}" 'NR > start && index($0, "\"$tmp_snap/$SERVICE_STATES_BOOT_RECORD\" \"$tmp_snap/snapshot-transition.state\"") { print NR; exit }' "$update")
[[ -n $write_line && -n $ledger_line && -n $load_line && -n $sync_line &&
   $ledger_line -lt $write_line && $write_line -lt $load_line && $load_line -lt $sync_line ]] ||
    fail "update.sh does not record the boot between writing and loading the ledger, before it is made durable"
# A resumed stage keeps the record with the ledger it belongs to.
grep -Fq 'service-states.boot-id) [[ -f $entry && ! -L $entry ]] && continue ;;' \
    "$repo_root/deploy/release-transaction-guard.sh" || fail "stage reset does not keep the boot record"

saved_boot=11111111-2222-4333-8444-555555555555
other_boot=99999999-8888-4777-8666-555555555555

# Behaviour, per script: the extracted functions with a fixture systemctl.
run_case() {
    local script=$1 name=$2 want=$3 needle=$4
    shift 4
    local status=0
    (
        set -euo pipefail
        # Fixture defaults; each case overrides with name=value arguments.
        FX_BOOT=$other_boot FX_RECORD=$saved_boot FX_SAVED_FW_ENABLED=enabled FX_SAVED_FW_STATE=inactive
        FX_FW_ENABLED=enabled FX_FW_ACTIVE=1 FX_RESULT=success FX_SUB=exited FX_ENTERED=4200000 FX_FINISHED=9100000
        FX_POLICY=1 FX_SAVED_AGENT=active FX_AGENT_ACTIVE=1 FX_RECORD_RAW= FX_CALLS=1
        for assignment in "$@"; do declare "$assignment"; done
        case_root=$work/$name.${script//\//_}
        mkdir -p -- "$case_root/snapshot"
        die() { printf '!! %s\n' "$*" >&2; exit 1; }
        service_state_is_active_like() { case "$1" in active|activating|reloading|refreshing) return 0 ;; *) return 1 ;; esac; }
        systemctl() {
            case "$*" in
                'is-active --quiet celikpanel-agent.service') [[ $FX_AGENT_ACTIVE == 1 ]] ;;
                'is-active --quiet celikpanel-panel.service') return 0 ;;
                'is-active --quiet celikpanel-firewall-restore.service') [[ $FX_FW_ACTIVE == 1 ]] ;;
                'is-enabled celikpanel-firewall-restore.service') printf '%s\n' "$FX_FW_ENABLED"; [[ $FX_FW_ENABLED == enabled ]] ;;
                'show --property=Result --value celikpanel-firewall-restore.service') printf '%s\n' "$FX_RESULT" ;;
                'show --property=SubState --value celikpanel-firewall-restore.service') printf '%s\n' "$FX_SUB" ;;
                'show --property=ActiveEnterTimestampMonotonic --value celikpanel-firewall-restore.service') printf '%s\n' "$FX_ENTERED" ;;
                'show --property=FinishTimestampMonotonic --value') printf '%s\n' "$FX_FINISHED" ;;
                *) printf 'unexpected systemctl %s\n' "$*" >&2; return 99 ;;
            esac
        }
        KERNEL_BOOT_ID_FILE=$case_root/boot_id
        FIREWALL_POLICY_FILE=$case_root/firewall.nft
        SERVICE_STATES_BOOT_RECORD=service-states.boot-id
        firewall_boot_activation_reported=0
        printf '%s\n' "$FX_BOOT" > "$KERNEL_BOOT_ID_FILE"
        [[ $FX_POLICY != 1 ]] || printf 'table inet celikpanel_fw {}\n' > "$FIREWALL_POLICY_FILE"
        ledger=$case_root/snapshot/service-states.tsv
        : > "$ledger"
        if [[ -n $FX_RECORD_RAW ]]; then
            printf '%b' "$FX_RECORD_RAW" > "$case_root/snapshot/service-states.boot-id"
        elif [[ $FX_RECORD == symlink ]]; then
            printf '%s\n' "$saved_boot" > "$case_root/elsewhere"
            ln -s -- "$case_root/elsewhere" "$case_root/snapshot/service-states.boot-id"
        elif [[ $FX_RECORD != none ]]; then
            printf '%s\n' "$FX_RECORD" > "$case_root/snapshot/service-states.boot-id"
        fi
        declare -A saved_enabled_states=(
            [celikpanel-agent.service]=enabled [celikpanel-panel.service]=enabled
            [celikpanel-firewall-restore.service]=$FX_SAVED_FW_ENABLED)
        declare -A saved_active_states=(
            [celikpanel-agent.service]=$FX_SAVED_AGENT [celikpanel-panel.service]=active
            [celikpanel-firewall-restore.service]=$FX_SAVED_FW_STATE)
        for fn in "${functions[@]}"; do eval "$(extract "$repo_root/$script" "$fn")"; done
        load_saved_service_states_boot "$ledger"
        for (( call = 0; call < FX_CALLS; call++ )); do verify_saved_runtime_states; done
    ) > "$work/out" 2> "$work/err" || status=$?
    if [[ $want == accept ]]; then
        [[ $status == 0 ]] || fail "$script $name: refused ($status): $(cat "$work/err")"
    else
        [[ $status != 0 ]] || fail "$script $name: accepted, want refusal"
    fi
    [[ -z $needle ]] || grep -Fq -- "$needle" "$work/out" "$work/err" ||
        fail "$script $name: output lacks '$needle': $(cat "$work/out" "$work/err")"
    [[ ! -s $work/err ]] || ! grep -Fq 'unexpected systemctl' "$work/err" ||
        fail "$script $name: unexpected systemctl call: $(cat "$work/err")"
    printf '%s\n' "$(cat "$work/out")" > "$work/last-out"
}

inactive_refusal='saved inactive-like service became active: celikpanel-firewall-restore.service'
accepted='was started by a boot after the service states were saved'
for script in "${scripts[@]}"; do
    # The lab's case: a boot since the save ran the unit successfully.
    run_case "$script" boot-run accept "$accepted"
    # Said once per invocation, though the scripts verify more than once.
    run_case "$script" said-once accept "$accepted" FX_CALLS=3
    [[ $(grep -cF "$accepted" "$work/last-out") == 1 ]] || fail "$script: acceptance line repeated"
    # Without a boot in between, the refusal stays exactly as it was.
    run_case "$script" same-boot refuse "$inactive_refusal" FX_BOOT=$saved_boot
    # A ledger without a boot record (an earlier release's) proves no boot.
    run_case "$script" no-record refuse "$inactive_refusal" FX_RECORD=none
    # The unit's own run must have succeeded, as a finished oneshot.
    run_case "$script" result-not-success refuse "$inactive_refusal" FX_RESULT=exit-code
    run_case "$script" not-exited refuse "$inactive_refusal" FX_SUB=running
    # Enabled as saved, and enabled now: only an enabled unit runs at boot.
    run_case "$script" saved-disabled refuse "$inactive_refusal" FX_SAVED_FW_ENABLED=disabled
    run_case "$script" now-disabled refuse "$inactive_refusal" FX_FW_ENABLED=disabled
    # Its condition: the policy file exists.
    run_case "$script" no-policy refuse "$inactive_refusal" FX_POLICY=0
    # Started after this boot's startup finished (by anything, a recovery run
    # included), or while startup has not finished: not the boot's own run.
    run_case "$script" started-after-boot refuse "$inactive_refusal" FX_ENTERED=9100001
    run_case "$script" boot-unfinished refuse "$inactive_refusal" FX_FINISHED=0
    run_case "$script" never-entered refuse "$inactive_refusal" FX_ENTERED=0
    # An unreadable boot id proves nothing.
    run_case "$script" boot-unreadable refuse "$inactive_refusal" FX_BOOT=not-a-boot-id
    # Unchanged rules: saved inactive and still inactive passes; saved active
    # and now inactive refuses even after a boot; the agent's and the panel's
    # checks take no boot exception.
    run_case "$script" still-inactive accept '' FX_FW_ACTIVE=0
    run_case "$script" active-now-inactive refuse 'saved active-like service is not active: celikpanel-firewall-restore.service' \
        FX_SAVED_FW_STATE=active FX_FW_ACTIVE=0
    run_case "$script" agent-no-exception refuse 'saved inactive-like service became active: celikpanel-agent.service' \
        FX_SAVED_AGENT=inactive
    # A record that is present but not one canonical boot id line is refused.
    run_case "$script" record-malformed refuse 'service-state boot record is unsafe' FX_RECORD_RAW='not-a-boot\n'
    run_case "$script" record-two-lines refuse 'service-state boot record is unsafe' FX_RECORD_RAW="$saved_boot\n$saved_boot\n"
    run_case "$script" record-no-newline refuse 'service-state boot record is not canonical' FX_RECORD_RAW="${saved_boot}x"
    run_case "$script" record-uppercase refuse 'service-state boot record is not canonical' FX_RECORD_RAW='11111111-2222-4333-8444-55555555555A\n'
    run_case "$script" record-symlink refuse 'service-state boot record is unsafe' FX_RECORD=symlink
    printf 'PASS: %s saved runtime boot rule\n' "$script"
done
printf 'saved runtime boot rule: PASS\n'
