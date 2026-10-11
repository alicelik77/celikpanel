#!/bin/bash
# set11: the root SHA256SUMS over every file of the evidence folder (generated last), then verified; the per-run driver
# sums verified too.
# Intake 2026-10-11: the checkout path is shown as <repo>; as run, the find below excluded every file NAMED SHA256SUMS,
# so the five per-run driver sums were not listed in the root file. The root list regenerated at intake includes them
# (only ./SHA256SUMS itself is left out).
E="<repo>/deploy/e2e/release-recovery/evidence/set11-20261011"
cd "$E" && rm -f SHA256SUMS && find . -type f ! -path ./SHA256SUMS -print0| sort -z | xargs -0 sha256sum | sed 's#  \./#  #' > /var/tmp/cp-set11-run/SHA256SUMS.tmp && mv /var/tmp/cp-set11-run/SHA256SUMS.tmp SHA256SUMS
wc -l < SHA256SUMS; sha256sum -c --quiet SHA256SUMS && echo "root SHA256SUMS verified"
for d in */*/driver; do if [ -f "$d/SHA256SUMS" ]; then ( cd "$d" && sha256sum -c --quiet SHA256SUMS ) && echo "$d sums ok"; else echo "$d has no driver sums"; fi; done
grep -c $'\r' SHA256SUMS || true
