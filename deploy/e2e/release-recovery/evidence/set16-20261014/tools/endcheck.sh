#!/bin/bash
# set16: the end of the run: time, what is left and how large (reads only).
echo "date_utc_end=$(date -u +%FT%TZ)"
echo "qemu=$(pgrep -c qemu-system || true)"
pgrep -af 'run-set16|set16_trial|lab.py|cold-load' | grep -v pgrep || echo "no set16 process runs"
du -sh /var/tmp/cp-set16 /var/tmp/cp-release-drill-s16-* 2>/dev/null
for d in f0157e6c8f1614cff176821172f8a9067493e625 6f290944c1c88d33c994a81c517900ab231fed3c d4c0acd80e8a34f380ba40db48533621685209d4 632fa829c57315d9c1c805ab60f1e68c9d094154 b1191c38866a60a59c1dd0037e9a136d812fbc59; do du -sh /var/tmp/cp-pair-accept/dist/$d-acceptance-license; done
ls /var/tmp/cp-set16/cp-upd1-build/
