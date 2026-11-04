# Recheck round: fresh u22 then d12 (staggered 30 s), each get.sh install + one-server setup with BIND. Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/lab/kit
( CELL=u22 IP=192.0.3.191 Z=u22.test VER=v0.1.0-alpha.122 OS=ubuntu2204 MEM=4096 SHORT=boston DNSENGINE=bind bash $A/singleR.sh > $A/singleRU22.out 2>&1 ) &
sleep 30
( CELL=d12 IP=192.0.3.190 Z=d12.test VER=v0.1.0-alpha.122 OS=debian12 MEM=4096 SHORT=frankfurt DNSENGINE=bind bash $A/singleR.sh > $A/singleRD12.out 2>&1 ) &
wait
echo "both done $(date -u +%T)"
