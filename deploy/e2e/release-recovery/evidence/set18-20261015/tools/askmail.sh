#!/bin/bash
# set18 run note H3: the tool's trigger for the mail record compared the run's phase with the step id (the waiting run
# reports phase "access_dns"); the coordinator asks the driver for the same preparation through the same handover.
k=lab-mail-a-$(date +%s%3N)
date -u +%FT%T.%3NZ > /var/tmp/cp-set18/hand/h18b-setup/prep-request-$k
echo "$(date -u +%FT%TZ) coordinator wrote prep-request-$k (H3)" >> /var/tmp/cp-set18/progress.txt
for i in $(seq 1 60); do [ -e /var/tmp/cp-set18/hand/h18b-setup/prep-done-$k.json ] && break; sleep 1; done
cat /var/tmp/cp-set18/hand/h18b-setup/prep-done-$k.json | head -c 1500
