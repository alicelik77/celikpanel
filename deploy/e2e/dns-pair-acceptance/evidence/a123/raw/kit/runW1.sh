# Wave 1: fresh Debian 12 (d12) and Ubuntu 22.04 (u22), PowerDNS first (refusal), then BIND. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit
( CELL=u22 IP=192.0.3.191 Z=u22.test OS=ubuntu2204 SHORT=boston PDNSFIRST=1 bash $A/singleX.sh > $A/singleU22.out 2>&1 ) &
sleep 30
( CELL=d12 IP=192.0.3.190 Z=d12.test OS=debian12 SHORT=frankfurt PDNSFIRST=1 bash $A/singleX.sh > $A/singleD12.out 2>&1 ) &
wait
echo "wave1 done $(date -u +%T)"
