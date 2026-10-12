#!/bin/bash
ls /var/tmp/cp-release-drill-s14-ub-a/evidence/
node=$(ls /var/tmp/cp-release-drill-s14-ub-a/evidence/ | head -n 1)
bash <redacted> ub s14-ub-a "$node" ubuntu/run-a h14ub
