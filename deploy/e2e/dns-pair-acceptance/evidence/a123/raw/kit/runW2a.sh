# Wave 2a on 894fd63c9: fresh Debian 13 (d13: PowerDNS, case 4 DB write-lock hold during setup) and the fresh Ubuntu 22.04 rerun (u22r: PowerDNS refusal, then BIND). Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit
( CELL=u22r IP=192.0.3.191 Z=u22r.test OS=ubuntu2204 SHORT=boston PDNSFIRST=1 bash $A/singleX.sh > $A/singleU22r.out 2>&1 ) &
sleep 30
( CELL=d13 IP=192.0.3.192 Z=d13.test OS=debian13 SHORT=frankfurt HOLDDB=1 HOLDSEC=60 HOLDAFTER=5 bash $A/singleX.sh > $A/singleD13.out 2>&1 ) &
wait
echo "wave2a done $(date -u +%T)"
