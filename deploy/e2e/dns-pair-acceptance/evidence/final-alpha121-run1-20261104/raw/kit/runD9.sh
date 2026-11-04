A=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab
PREVV=v0.1.0-alpha.117 NEXTV=v0.1.0-alpha.119 bash $A/portalF.sh up v0.1.0-alpha.119 > $A/portalnet-up.out 2>&1
CELL=d117 IP=192.0.3.190 Z=d117.test VER=v0.1.0-alpha.117 OS=debian13 MEM=4096 SHORT=frankfurt TRUST=1 bash $A/singleA.sh > $A/singleD.out 2>&1
CELL=d117 IP=192.0.3.190 Z=d117.test bash $A/R9.sh > $A/R9D.out 2>&1
