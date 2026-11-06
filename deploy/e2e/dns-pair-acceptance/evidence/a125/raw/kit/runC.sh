A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit
ok() { sed -n '/^== after/,$p' $A/$1 | grep -q 'v0.1.0-alpha.125' || { echo "STOP: $1 did not reach 125 $(date -u +%T)"; exit 1; }; }
net() { dmesg | grep -q "sub channel open failed" && { echo "STOP: host network adapter failed again (dmesg) $(date -u +%T)"; exit 2; }; timeout 15 curl -s -o /dev/null https://deb.debian.org/debian/dists/trixie/Release || echo "warn: host cannot reach deb.debian.org $(date -u +%T)"; }
net
CELL=d5 IP=192.0.3.193 Z=d5.test VER=v0.1.0-alpha.125 OS=debian12 MEM=2560 SHORT=frankfurt DNSENGINE=bind TRUST=1 bash $A/single124.sh > $A/singleD5.out 2>&1; grep -E "^(succeeded|failed)" $A/singleD5.out | tail -1
net; bash $A/pre.sh > $A/pre.out 2>&1
net; bash $A/upd.sh main > $A/C1main.out 2>&1; ok C1main.out
net; bash $A/upd.sh backup > $A/C1backup.out 2>&1; ok C1backup.out
net; bash $A/post3.sh > $A/post3.out 2>&1
net; bash $A/post4.sh > $A/post4.out 2>&1
net; bash $A/case2.sh > $A/case2.out 2>&1
echo "runC done $(date -u +%T)"
