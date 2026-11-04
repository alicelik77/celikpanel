A=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab
bash $A/portalF.sh up v0.1.0-alpha.118 > $A/portalnet-up.out 2>&1
CELL=u117 IP=192.0.3.191 Z=u117.test VER=v0.1.0-alpha.117 OS=ubuntu2404 MEM=4096 SHORT=boston TRUST=1 bash $A/singleA.sh > $A/singleB.out 2>&1
