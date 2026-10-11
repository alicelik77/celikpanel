#!/bin/bash
bash <scratchpad>/set13/tools/mkov.sh c set13_trial.py guest_set13_native.py run-set13.sh test_set13_trial.py > /var/tmp/cp-set13-run/logs/mkov-c.txt 2>&1; echo "mkov c rc=$?"
bash <scratchpad>/set13/tools/suite.sh c
sha256sum /var/tmp/cp-set12-run/generated/worker_fixture_origin.py /var/tmp/cp-set12-run/generated/worker_fixture_origin.json
