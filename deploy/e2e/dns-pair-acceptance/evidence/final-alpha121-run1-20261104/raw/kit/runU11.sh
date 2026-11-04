A=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab
PREVV=v0.1.0-alpha.120 NEXTV=v0.1.0-alpha.121 bash $A/portalF.sh up v0.1.0-alpha.121 > $A/portalnet-up.out 2>&1
CELL=u120 IP=192.0.3.191 Z=u120.test VER=v0.1.0-alpha.120 OS=ubuntu2404 MEM=4096 SHORT=boston TRUST=1 DNSENGINE=bind bash $A/singleA.sh > $A/singleU.out 2>&1
bash $A/R11U.sh > $A/R11U.out 2>&1
