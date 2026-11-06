A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit
net() { dmesg | grep -q "sub channel open failed" && { echo "STOP: host network adapter failed again (dmesg) $(date -u +%T)"; exit 2; }; }
net; bash $A/case2m.sh > $A/case2m.out 2>&1
net; CELL=d5 IP=192.0.3.193 Z=d5.test VER=v0.1.0-alpha.125 OS=debian12 MEM=2560 SHORT=frankfurt DNSENGINE=bind TRUST=1 bash $A/single124.sh > $A/singleD5b.out 2>&1; grep -E "^(succeeded|failed)" $A/singleD5b.out | tail -1
net; CELL=d5 IP=192.0.3.193 Z=d5.test bash $A/upd.sh main > $A/C1d5.out 2>&1
echo "runC3 done $(date -u +%T)"
