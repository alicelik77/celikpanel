#!/usr/bin/env bash
# Shared stand-in for systemctl that models Debian/Ubuntu Certbot renewal the
# way the native units behave: certbot.timer is Persistent= with a randomized
# delay, so starting it may start certbot.service at once (a catch-up run that
# reads "activating" while it works and then "inactive" or "failed"); a
# renewal can already be running or have failed before an update. The fixture
# records every call and any action the updater must never take on the
# owner's renewal (stop, start, restart, kill or reset-failed of a service).
# Sourced by deploy/test-panel-tls-snapshot-contract.sh and
# deploy/test-recovery-renewal-pause-contract.sh; it is not a test itself.
# Debian/Ubuntu Certbot yenilemesini yerel birimler gibi taklit eder:
# zamanlayıcı başlatılınca hizmet hemen çalışabilir. Güncelleyicinin sahibin
# yenilemesine asla yapmaması gereken her eylem kaydedilir.
if [[ ${BASH_SOURCE[0]} == "$0" ]]; then
    printf 'SKIP: %s is a fixture sourced by contract tests\n' "${0##*/}"
    exit 0
fi

# certbot_fixture_init DIR: fresh state (timer enabled and active, renewal idle).
certbot_fixture_init() {
    CERTBOT_FIXTURE=${1:?fixture directory required}
    rm -rf -- "$CERTBOT_FIXTURE"
    install -d -m 0700 -- "$CERTBOT_FIXTURE"
    CERTBOT_FIXTURE_TIMER_CONTENT='[Unit]\nDescription=Run certbot twice daily\n\n[Timer]\nOnCalendar=*-*-* 00,12:00:00\nRandomizedDelaySec=43200\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n'
    certbot_fixture_set timer-active active
    certbot_fixture_set timer-enabled enabled
    certbot_fixture_set run inactive
    certbot_fixture_set trigger none
    certbot_fixture_set finish-after 0
    certbot_fixture_set finish-state inactive
    certbot_fixture_set seq 1
    certbot_fixture_set invocation "$(printf '%032x' 1)"
    : >"$CERTBOT_FIXTURE/calls"
    : >"$CERTBOT_FIXTURE/violations"
}
certbot_fixture_get() { cat -- "$CERTBOT_FIXTURE/$1"; }
certbot_fixture_set() { printf '%s\n' "$2" >"$CERTBOT_FIXTURE/$1"; }
certbot_fixture_new_invocation() {
    local n
    n=$(certbot_fixture_get seq)
    n=$((n + 1))
    certbot_fixture_set seq "$n"
    certbot_fixture_set invocation "$(printf '%032x' "$n")"
}
# certbot_fixture_running STATE POLLS FINAL: a renewal is in progress and
# finishes (to FINAL) after POLLS reads of its state; 0 polls never finishes.
certbot_fixture_running() {
    certbot_fixture_new_invocation
    certbot_fixture_set run "$1"
    certbot_fixture_set finish-after "$2"
    certbot_fixture_set finish-state "$3"
}
certbot_fixture_violations() { cat -- "$CERTBOT_FIXTURE/violations"; }

systemctl() {
    local command=${1:?command required} property= argument unit left
    shift
    unit=${*: -1}
    printf '%s %s\n' "$command" "$unit" >>"$CERTBOT_FIXTURE/calls"
    case "$command" in
        show)
            for argument in "$@"; do
                case "$argument" in --property=*) property=${argument#--property=} ;; esac
            done
            case "$property:$unit" in
                LoadState:certbot.timer|LoadState:certbot.service) printf 'loaded\n' ;;
                LoadState:certbot-renew.timer|LoadState:certbot-renew.service) printf 'not-found\n' ;;
                InvocationID:certbot.service) certbot_fixture_get invocation ;;
                InvocationID:*) printf '\n' ;;
                *) return 1 ;;
            esac
            ;;
        is-enabled)
            case "$unit" in
                certbot.timer) certbot_fixture_get timer-enabled ;;
                certbot.service) printf 'static\n' ;;
                *) printf 'not-found\n'; return 1 ;;
            esac
            ;;
        is-active)
            case "$unit" in
                certbot.timer)
                    certbot_fixture_get timer-active
                    [[ $(certbot_fixture_get timer-active) == active ]] || return 3
                    ;;
                certbot.service)
                    left=$(certbot_fixture_get finish-after)
                    if (( left > 0 )); then
                        left=$((left - 1))
                        certbot_fixture_set finish-after "$left"
                        (( left > 0 )) || certbot_fixture_set run "$(certbot_fixture_get finish-state)"
                    fi
                    certbot_fixture_get run
                    [[ $(certbot_fixture_get run) == active ]] || return 3
                    ;;
                *) printf 'inactive\n'; return 3 ;;
            esac
            ;;
        cat)
            [[ $unit == certbot.timer ]] || return 1
            printf '%b' "$CERTBOT_FIXTURE_TIMER_CONTENT"
            ;;
        stop)
            case "$unit" in
                certbot.timer) certbot_fixture_set timer-active inactive ;;
                *)
                    printf 'stop %s\n' "$unit" >>"$CERTBOT_FIXTURE/violations"
                    certbot_fixture_set run inactive
                    certbot_fixture_set finish-after 0
                    ;;
            esac
            ;;
        start)
            case "$unit" in
                certbot.timer)
                    certbot_fixture_set timer-active active
                    # Persistent=true: a missed elapse starts the renewal now.
                    case "$(certbot_fixture_get trigger)" in
                        none) ;;
                        activating|active) certbot_fixture_running "$(certbot_fixture_get trigger)" 0 inactive ;;
                        finished-ok) certbot_fixture_new_invocation; certbot_fixture_set run inactive ;;
                        finished-failed) certbot_fixture_new_invocation; certbot_fixture_set run failed ;;
                    esac
                    ;;
                *) printf 'start %s\n' "$unit" >>"$CERTBOT_FIXTURE/violations" ;;
            esac
            ;;
        enable) [[ $unit != certbot.timer ]] || certbot_fixture_set timer-enabled enabled ;;
        disable) [[ $unit != certbot.timer ]] || certbot_fixture_set timer-enabled disabled ;;
        restart|kill|reset-failed|try-restart|reload-or-restart)
            printf '%s %s\n' "$command" "$unit" >>"$CERTBOT_FIXTURE/violations"
            return 1
            ;;
        *) return 1 ;;
    esac
}

# certbot_fixture_capture TEST_ROOT NAME: an absent-TLS snapshot of the current
# scheduler state with its five-row service ledger; prints the panel-tls root.
certbot_fixture_capture() {
    local test_root=${1:?test root required} name=${2:?name required} dir
    dir=$test_root/certbot-$name
    rm -rf -- "$dir"
    install -d -m 0700 -- "$dir" "$dir/panel-tls"
    printf '%s\t%s\t%s\n' \
        celikpanel-agent.service enabled active \
        celikpanel-panel.service enabled active \
        celikpanel-firewall-restore.service enabled inactive >"$dir/service-states.tsv"
    chmod 0600 -- "$dir/service-states.tsv"
    panel_tls_capture_scheduler_states_to_service_ledger "$dir/service-states.tsv" >&2 || return 1
    panel_tls_snapshot_capture "$dir/panel-tls" "$test_root/tls" \
        "$test_root/agent/panel-certificate-activation.json" \
        "$test_root/etc/letsencrypt/renewal-hooks/deploy/celikpanel-panel-cert" \
        "$dir/service-states.tsv" || return 1
    printf '%s\n' "$dir/panel-tls"
}
