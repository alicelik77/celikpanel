#!/bin/bash
# set13: (1) look for what the mis-quoted inline command of 09:4xZ may have created (files named *.set13-restore at /
# and in the repository root); (2) put set12's two generated files, which set13's offline suite rewrote at 09:37:25Z,
# back to the bytes staged in set12's evidence (build/generated/), CR bytes removed.
set -u
echo "== at $(date -u +%FT%TZ)"
ls -la --time-style=full-iso /.set13-restore /*.set13-restore 2>&1 | head
ls -la --time-style=full-iso "<repo>/"*.set13-restore 2>&1 | head
E='<repo>/deploy/e2e/release-recovery/evidence/set12-20261011/build/generated'
G=/var/tmp/cp-set12-run/generated
[ "${1:-}" = restore ] || exit 0
for f in worker_fixture_origin.py worker_fixture_origin.json; do
  tr -d '\r' < "$E/$f" > "$G/$f.set13-restore" && chmod 0644 "$G/$f.set13-restore" && mv "$G/$f.set13-restore" "$G/$f"
done
sha256sum "$G/worker_fixture_origin.py" "$G/worker_fixture_origin.json"
( cd "$E" && tr -d '\r' < worker_fixture_origin.json | sha256sum )
ls -la --time-style=full-iso "$G"
