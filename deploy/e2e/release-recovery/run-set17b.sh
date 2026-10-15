#!/usr/bin/env bash
# set17b wrapper for the Linux QEMU host (archlinux), run as root: the update page through a LONG restart outage
# (lab preparation: the update's own worker held 150 s at completion-pending while the Panel is stopped; see
# guest_set17b_restart_delay.py), on disposable Debian 13 guests.
#
#   run-set17b.sh dry-run CELL ARTIFACTS_JSON LAB_NAME
#       Validate the plan without any guest (no lab is created).
#   run-set17b.sh cell CELL ARTIFACTS_JSON LAB_NAME SSH_PORT [LOCAL_PORT]
#       Prepare and start a NEW lab /var/tmp/cp-release-drill-LAB_NAME (lab.py accepts no other place), run the one
#       cell on its node, then stop the lab (disks and evidence retained).
#
# CELL: set17b-upd86-debian13-good          (ARTIFACTS_JSON from `build-set17-artifacts.sh --second UPDATE_ARTIFACTS_JSON`)
#       set17b-upd1-debian13-good-control   (ARTIFACTS_JSON from `build-set17-artifacts.sh --baseline-ref v0.1.0-alpha.85 SOURCE`)
# One cell per new lab. SET17_NEIGHBOUR_GATE and then SET17_DISK_GATE, when set, are programs that must exit 0
# immediately before the guests are started; otherwise no guest is started. SET12_HAND opens the browser window
# "page" (required for these cells; under $SET12_RUN_ROOT/hand).
set -euo pipefail
HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
IMAGES=${UPD1_IMAGE_CACHE:-/var/tmp/cp-v3n28/images}
LAB=(python3 "$HERE/lab.py")
DRIVER=(python3 "$HERE/set17b_trial.py")

usage() { sed -n '2,17p' "${BASH_SOURCE[0]}" >&2; exit 2; }
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
        [[ -n ${SET12_HAND:-} ]] || { echo "set17b cells need the browser window (SET12_HAND)" >&2; exit 2; }
        root=/var/tmp/cp-release-drill-$name
        [[ ! -e $root ]] || { echo "lab $root exists; every cell needs a NEW lab" >&2; exit 2; }
        "${DRIVER[@]}" plan --cell "$cell" --artifacts "$artifacts" --work-root "$root" > /dev/null
        "${LAB[@]}" prepare --work-root "$root" --image-cache "$IMAGES" --ssh-port "$port" --platform debian13-arch --execute
        if [[ -n ${SET17_NEIGHBOUR_GATE:-} ]]; then
            bash "$SET17_NEIGHBOUR_GATE" "$cell $name" || { echo "SET17B cell=$cell lab=$root NOT STARTED: another measurement's guest is running" >&2; exit 75; }
        fi
        if [[ -n ${SET17_DISK_GATE:-} ]]; then
            bash "$SET17_DISK_GATE" "$cell $name (guest start)" || { echo "SET17B cell=$cell lab=$root NOT STARTED: the disk gate refused the guest start" >&2; exit 75; }
        fi
        "${LAB[@]}" start --work-root "$root" --execute
        "${LAB[@]}" status --work-root "$root"
        status=0
        "${DRIVER[@]}" run --cell "$cell" --artifacts "$artifacts" --work-root "$root" --local-port "$local_port" \
            --execute || status=$?
        "${LAB[@]}" stop --work-root "$root" --execute || true
        echo "SET17B cell=$cell lab=$root driver_exit=$status evidence=$root/evidence/*/upd1"
        exit "$status"
        ;;
    *) usage ;;
esac
