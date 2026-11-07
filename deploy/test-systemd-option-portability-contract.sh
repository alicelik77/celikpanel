#!/usr/bin/env bash
# The oldest supported system is Ubuntu 22.04 with systemd 249. Its systemctl
# knows `--kill-who=` but not `--kill-whom=` (that spelling arrived in systemd
# 252; newer versions still accept `--kill-who=`). Shipped scripts and the
# files the Agent writes must use the spelling every supported systemd knows:
# with `--kill-whom=` the update stopped at "panel could not be frozen" on
# Ubuntu 22.04 and nginx never reopened its traffic logs after a rotation
# (alpha.129 lab, a128-other-systems).
#
# Desteklenen en eski sistem Ubuntu 22.04 (systemd 249): `--kill-whom=`
# bilinmez; her desteklenen systemd `--kill-who=` biçimini bilir.
set -euo pipefail

root=${1:-.}
cd "$root"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

shipped=(install.sh update.sh rollback.sh deploy/*.sh download-portal/get.sh)
hits=$(grep -nF -- '--kill-whom' "${shipped[@]}" 2>/dev/null | grep -v '^deploy/test-systemd-option-portability-contract.sh:' || true)
[[ -z "$hits" ]] || fail "systemctl --kill-whom= is unknown to systemd 249 (Ubuntu 22.04); use --kill-who=:"$'\n'"$hits"

go_hits=$(grep -rnF --include='*.go' -- '--kill-whom' cmd internal 2>/dev/null || true)
[[ -z "$go_hits" ]] || fail "systemctl --kill-whom= in Go sources (written to the server or run there); use --kill-who=:"$'\n'"$go_hits"

grep -qF 'systemctl kill --kill-who=all --signal=SIGSTOP "$unit"' update.sh \
    || fail "update.sh no longer freezes a service with --kill-who=all"

echo "systemd option portability contract passed"
