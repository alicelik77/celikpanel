#!/bin/bash
export PYTHONDONTWRITEBYTECODE=1 SET12_RUN_ROOT=/var/tmp/cp-set18
cd /var/tmp/cp-set18/harness-b/deploy/e2e/release-recovery
python3 -I -B -c 'import sys; sys.path.insert(0,"."); import set18_fixture as f; print(sorted(f.out.BASELINE_REFS)); m=f.out._MODULES[f.fx12.ORIGIN_NAME]; print([p["current"] for p in m.RELEASE_POLICIES][-6:]); print(m.allowed_policy({"version":"v0.1.0-alpha.88","current":88,"previous":87,"previous_version":"v0.1.0-alpha.87"}))'
bash -n build-set18-artifacts.sh && echo syntax-ok
