#!/usr/bin/env bash
# set17 wrapper for the Linux QEMU host (archlinux), run as root: the pre-publication measurement of the
# v0.1.0-alpha.86 candidate (commit 313f3eea2) on disposable guests: installed fresh (cell set17-debian13), reached by
# the owner's update from alpha.85 rebuilt from its tag (cells upd1-<debian13|ubuntu>-<good|defective>), and updated
# itself to a v0.1.0-alpha.87 label (cell upd86-debian13-good).
#
#   run-set17.sh dry-run CELL ARTIFACTS_JSON LAB_NAME
#       Validate the plan without any guest (no lab is created).
#   run-set17.sh cell CELL ARTIFACTS_JSON LAB_NAME SSH_PORT [LOCAL_PORT]
#       Prepare and start a NEW lab /var/tmp/cp-release-drill-LAB_NAME (lab.py accepts no other place), run the one
#       cell on its node, then stop the lab (disks and evidence retained).
#
# CELL: set17-debian13 | upd86-debian13-good   (ARTIFACTS_JSON from `build-set17-artifacts.sh --second UPDATE_ARTIFACTS_JSON`)
#       upd1-<debian13|ubuntu>-<good|defective>  (ARTIFACTS_JSON from `build-set17-artifacts.sh --baseline-ref v0.1.0-alpha.85 SOURCE`)
# One cell per new lab. SET17_NEIGHBOUR_GATE and then SET17_DISK_GATE, when set, are programs that must exit 0
# immediately before the guests are started; otherwise no guest is started. SET12_HAND, when set, opens the browser
# windows (set17_trial.py's handovers "page" and "ui", under $SET12_RUN_ROOT/hand).
set -euo pipefail
HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
IMAGES=${UPD1_IMAGE_CACHE:-/var/tmp/cp-v3n28/images}
LAB=(python3 "$HERE/lab.py")
DRIVER=(python3 "$HERE/set17_trial.py")

usage() { sed -n '2,18p' "${BASH_SOURCE[0]}" >&2; exit 2; }
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
        [[ $cell != *ubuntu* ]] || platform=ubuntu
        "${LAB[@]}" prepare --work-root "$root" --image-cache "$IMAGES" --ssh-port "$port" --platform "$platform" --execute
        if [[ -n ${SET17_NEIGHBOUR_GATE:-} ]]; then
            bash "$SET17_NEIGHBOUR_GATE" "$cell $name" || { echo "SET17 cell=$cell lab=$root NOT STARTED: another measurement's guest is running" >&2; exit 75; }
        fi
        if [[ -n ${SET17_DISK_GATE:-} ]]; then
            bash "$SET17_DISK_GATE" "$cell $name (guest start)" || { echo "SET17 cell=$cell lab=$root NOT STARTED: the disk gate refused the guest start" >&2; exit 75; }
        fi
        "${LAB[@]}" start --work-root "$root" --execute
        "${LAB[@]}" status --work-root "$root"
        status=0
        "${DRIVER[@]}" run --cell "$cell" --artifacts "$artifacts" --work-root "$root" --local-port "$local_port" \
            --execute || status=$?
        "${LAB[@]}" stop --work-root "$root" --execute || true
        echo "SET17 cell=$cell lab=$root driver_exit=$status evidence=$root/evidence/*/upd1"
        exit "$status"
        ;;
    *) usage ;;
esac
