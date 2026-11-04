#!/bin/bash
# Shared by the contract tests that stage executables under /run.
#
# GitHub's Ubuntu images sometimes mount /run without execute permission
# (alpha.122: "release-recovery is not executable: Permission denied" from
# systemd-analyze on two image versions, fine on a third). The product
# never executes from /run; the tests only stage their stubs there. So a
# test mounts its own small tmpfs, with execute allowed, over its working
# folder and removes it at the end. Without root the folder is used as is.
#
# Usage, right after mktemp:
#   source "$REPO_ROOT/deploy/test-private-tmpfs.sh"
#   mount_private_test_tmpfs "$TEST_ROOT"
#   trap 'remove_private_test_root "$TEST_ROOT"' EXIT

mount_private_test_tmpfs() {
    local dir=$1
    PRIVATE_TEST_TMPFS_DIR=
    [[ $(id -u) == 0 ]] || return 0
    mount -t tmpfs -o nosuid,nodev,mode=0700,size=512m celikpanel-test "$dir" 2>/dev/null || return 0
    PRIVATE_TEST_TMPFS_DIR=$dir
}

remove_private_test_root() {
    local dir=$1
    if [[ -n ${PRIVATE_TEST_TMPFS_DIR:-} && $PRIVATE_TEST_TMPFS_DIR == "$dir" ]]; then
        umount -- "$dir" 2>/dev/null || umount -l -- "$dir" 2>/dev/null || true
    fi
    rm -rf -- "$dir"
}
