#!/usr/bin/env bash
# set15 wrapper for the Linux QEMU host (archlinux), run as root: the pre-publication measurement of v0.1.0-alpha.84
# (commit 1636b73f0) on disposable guests: installed fresh (cells set15-*) and reached by the owner's update from
# alpha.83 rebuilt from its tag (cells upd1-debian13-good, upd1-debian13-defective).
#
#   run-set15.sh dry-run CELL ARTIFACTS_JSON LAB_NAME
#       Validate the plan without any guest (no lab is created).
#   run-set15.sh cell CELL ARTIFACTS_JSON LAB_NAME SSH_PORT [LOCAL_PORT]
#       Prepare and start a NEW lab /var/tmp/cp-release-drill-LAB_NAME, run the one cell on its node, then
#       stop the lab (disks and evidence retained).
#
# CELL: set15-debian13 | set15-ubuntu | set15-arch   (ARTIFACTS_JSON from `build-set15-artifacts.sh --fresh SOURCE`)
#       upd1-debian13-<good|defective>  (ARTIFACTS_JSON from `build-set15-artifacts.sh --baseline-ref v0.1.0-alpha.83 SOURCE`)
# One cell per new lab. SET15_NEIGHBOUR_GATE and then SET15_DISK_GATE, when set, are programs that must exit 0
# immediately before the guests are started; otherwise no guest is started. SET12_HAND, when set, opens the browser
# window (set15_trial.py's handover, under $SET12_RUN_ROOT/hand).
set -euo pipefail
HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
IMAGES=${UPD1_IMAGE_CACHE:-/var/tmp/cp-v3n28/images}
LAB=(python3 "$HERE/lab.py")
DRIVER=(python3 "$HERE/set15_trial.py")

usage() { sed -n '2,16p' "${BASH_SOURCE[0]}" >&2; exit 2; }
[[ $# -ge 1 ]] || usage
command=$1; shift
case $command in
    dry-run)
        [[ $# -eq 3 ]] || usage
        exec "${DRIVER[@]}" plan --cell "$1" --artifacts "$2" --work-root "/var/tmp/cp-release-drill-$3" --dry-run
        ;;
    cell)
        [[ $# -ge 4 && $# -le 5 ]] || usage
        cell=$1 artifacts=$2 name=$3 port=$4 local_port=${5:-18443}
        [[ $EUID -eq 0 ]] || { echo "run as root (fixture signing keys are root-only)" >&2; exit 2; }
        root=/var/tmp/cp-release-drill-$name
        [[ ! -e $root ]] || { echo "lab $root exists; every cell needs a NEW lab" >&2; exit 2; }
        "${DRIVER[@]}" plan --cell "$cell" --artifacts "$artifacts" --work-root "$root" > /dev/null
        platform=debian13-arch
        [[ $cell != set15-ubuntu ]] || platform=ubuntu
        "${LAB[@]}" prepare --work-root "$root" --image-cache "$IMAGES" --ssh-port "$port" --platform "$platform" --execute
        if [[ -n ${SET15_NEIGHBOUR_GATE:-} ]]; then
            bash "$SET15_NEIGHBOUR_GATE" "$cell $name" || { echo "SET15 cell=$cell lab=$root NOT STARTED: another measurement's guest is running" >&2; exit 75; }
        fi
        if [[ -n ${SET15_DISK_GATE:-} ]]; then
            bash "$SET15_DISK_GATE" "$cell $name (guest start)" || { echo "SET15 cell=$cell lab=$root NOT STARTED: the disk gate refused the guest start" >&2; exit 75; }
        fi
        "${LAB[@]}" start --work-root "$root" --execute
        "${LAB[@]}" status --work-root "$root"
        status=0
        "${DRIVER[@]}" run --cell "$cell" --artifacts "$artifacts" --work-root "$root" --local-port "$local_port" \
            --execute || status=$?
        "${LAB[@]}" stop --work-root "$root" --execute || true
        echo "SET15 cell=$cell lab=$root driver_exit=$status evidence=$root/evidence/*/upd1"
        exit "$status"
        ;;
    *) usage ;;
esac
