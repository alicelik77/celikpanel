#!/bin/bash
export PYTHONDONTWRITEBYTECODE=1
J=<scratchpad>/set12/tools
R=/var/tmp/cp-set12-run
bash $J/mkov.sh $1 set12_fixture.py build-set12-artifacts.sh > $R/logs/mkov-$1.txt 2>&1 || { tail -n 5 $R/logs/mkov-$1.txt; exit 1; }
tail -n 6 $R/logs/mkov-$1.txt
cd $R/harness-$1/deploy/e2e/release-recovery && bash -n build-set12-artifacts.sh && echo "build script parses"
SET12_RUN_ROOT=$R python3 -I -c "import sys; sys.path.insert(0,'.'); import set12_fixture as f; print(sorted(f.out.BASELINE_REFS)); o=f.out._MODULES['upd1_worker_fixture_origin']; print(o.RELEASE_POLICIES[-1], o.HERE)"
nproc; free -g | sed -n 2p
