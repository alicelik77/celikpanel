# alpha.129 phase 3b: case 2 (Ubuntu 22.04 forced failure, fresh 129), then Arch for case 4 (PostgreSQL TLS first). Lab only.
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
source $A/env.sh
bash $A/k2rb.sh > /dev/null 2>&1; echo "case 2 rollback done $(ts)"
bash $A/rmcell.sh u22u
S=u22 OS=ubuntu2204 IPF=192.0.3.211 VER=v0.1.0-alpha.129 bash $A/freshV.sh > /dev/null 2>&1; echo "case 2 fresh done $(ts)"; bash $A/rmcell.sh u22f
CELLX=aru S=ar OS=arch IPU=192.0.3.222 VER=v0.1.0-alpha.129 bash $A/setupV.sh > /dev/null 2>&1; echo "arch setup done $(ts)"
CELL=aru Z=ar.test MIP=192.0.3.222 BIP=192.0.3.222 IP=192.0.3.222 bash $A/k4remote.sh postgresql setup open restart checks fail2ban pass last > /dev/null 2>&1; echo "case 4 arch done $(ts)"
