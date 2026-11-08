export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
adm GET "/api/v1/service/operation?active=1" --label svcact | sbrief 800
adm GET /api/v1/runtimes/node --label nodes | sbrief 600
pkill -f "a129lab/kit/c3node.sh"; echo killed c3node
