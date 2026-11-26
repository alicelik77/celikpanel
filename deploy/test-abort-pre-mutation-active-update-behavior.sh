#!/usr/bin/env bash
set -euo pipefail

candidate=${1:-deploy/abort-pre-mutation-active-update.sh}
[[ -f "$candidate" ]] || {
    printf 'FAIL: missing pre-mutation abort helper: %s\n' "$candidate" >&2
    exit 1
}

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

die() {
    printf '!! %s\\n' "$*" >&2
    exit 1
}

fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT
snapshot=20260731T032113Z-from-unknown-to-85564a4437689b6c8aac474b819068cfccfc8882-0b249fb139e656af82539bd1acf7564a
EXPECTED_STAGE=$fixture/stage
ACTIVE_SNAPSHOT=$snapshot
child=$EXPECTED_STAGE/$ACTIVE_SNAPSHOT
mkdir -p -- "$child"
printf 'pre-ledger\n' > "$child/snapshot-transition.state"
printf 'services\n' > "$child/service-states.tsv"
printf 'coordinators\n' > "$child/quiesce-coordinators.tsv"

# Execute the exact two production evidence functions without sourcing the
# helper's privileged top-level recovery path. The extraction fails closed if
# their adjacency or names change.
evidence_functions=$(
    awk '
        /^verify_expected_stage_database\(\) \{/ { emit = 1 }
        /^verify_exact_preledger_stage\(\) \{/ { exit }
        emit { print }
    ' "$candidate"
)
[[ "$evidence_functions" == *'verify_expected_stage_database() {'* &&
   "$evidence_functions" == *'verify_exact_preledger_stage_payload() {'* ]] \
    || fail "cannot extract exact production stage-evidence functions"
eval "$evidence_functions"

SNAPSHOT_ROOT=$fixture
EXPECTED_STAGE=$fixture/stage
ACTIVE_SNAPSHOT=$snapshot
child=$EXPECTED_STAGE/$ACTIVE_SNAPSHOT

RECOVERY_PROFILE=legacy-scaffold
EXPECTED_STAGE_DATABASE_SHA256=
verify_exact_preledger_stage_payload "$child" \
    || fail "legacy exact three-file payload was rejected"

# A stage written from alpha.150 on also holds the saved states' boot record.
boot_record=$child/service-states.boot-id
printf '%s\n' 0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0 > "$boot_record"
verify_exact_preledger_stage_payload "$child" \
    || fail "legacy payload with the saved states' boot record was rejected"
rm -- "$boot_record"
ln -s -- "$child/service-states.tsv" "$boot_record"
if (verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "a symbolic-link boot record was accepted"
fi
rm -- "$boot_record"
mkdir -- "$boot_record"
if (verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "a directory boot record was accepted"
fi
rmdir -- "$boot_record"
printf 'unexpected\n' > "$child/service-states.boot-id.tmp"
printf '%s\n' 0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0 > "$boot_record"
if (verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "an extra entry beside the boot record was accepted"
fi
rm -- "$child/service-states.boot-id.tmp" "$boot_record"

printf 'standalone snapshot bytes\n' > "$child/celikpanel.db"
chmod 0600 -- "$child/celikpanel.db"
if (verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "legacy profile accepted a staged database fourth file"
fi

RECOVERY_PROFILE=preledger-database-snapshot
EXPECTED_STAGE_DATABASE_SHA256=$(sha256sum "$child/celikpanel.db" | awk '{print $1}')
stage_database_identity=
verify_exact_preledger_stage_payload "$child" \
    || fail "database-snapshot exact four-file payload was rejected"
printf '%s\n' 0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0 > "$boot_record"
(stage_database_identity=; verify_exact_preledger_stage_payload "$child") \
    || fail "database-snapshot payload with the saved states' boot record was rejected"
rm -- "$boot_record"

EXPECTED_STAGE_DATABASE_SHA256=$(printf wrong | sha256sum | awk '{print $1}')
if (stage_database_identity=; verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "database-snapshot profile accepted the wrong database digest"
fi
EXPECTED_STAGE_DATABASE_SHA256=$(sha256sum "$child/celikpanel.db" | awk '{print $1}')

printf 'sidecar\n' > "$child/celikpanel.db-wal"
if (stage_database_identity=; verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "database-snapshot profile accepted a SQLite sidecar"
fi
rm -- "$child/celikpanel.db-wal"

printf 'unexpected\n' > "$child/unexpected-entry"
if (stage_database_identity=; verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "database-snapshot profile accepted an extra direct entry"
fi
rm -- "$child/unexpected-entry"

truncate -s 2147483649 "$child/celikpanel.db"
if (stage_database_identity=; verify_exact_preledger_stage_payload "$child") >/dev/null 2>&1; then
    fail "database-snapshot profile accepted a database larger than 2 GiB"
fi

empty_agent_state_function=$(
    awk '
        /^verify_empty_preledger_agent_state\(\) \{/ { emit = 1 }
        /^capture_active_marker_evidence\(\) \{/ { exit }
        emit { print }
    ' "$candidate"
)
[[ "$empty_agent_state_function" == *'verify_empty_preledger_agent_state() {'* ]] \
    || fail "cannot extract exact production empty agent-state function"
eval "$empty_agent_state_function"
AGENT_STATE_DIR=$fixture/agent-state
verify_empty_preledger_agent_state \
    || fail "absent pre-ledger agent-state directory was rejected"
mkdir -- "$AGENT_STATE_DIR"
verify_empty_preledger_agent_state \
    || fail "empty pre-ledger agent-state directory was rejected"
printf '{}\n' > "$AGENT_STATE_DIR/.service-mutations-initial-deadbeef.json"
marker=$fixture/active-marker
printf 'active\n' > "$marker"
if (verify_empty_preledger_agent_state) >/dev/null 2>&1; then
    fail "partial initial mutation stage was accepted"
fi
[[ -f "$marker" ]] || fail "partial agent-stage rejection removed the active marker"

printf 'PASS: pre-mutation active update abort behavior\n'
