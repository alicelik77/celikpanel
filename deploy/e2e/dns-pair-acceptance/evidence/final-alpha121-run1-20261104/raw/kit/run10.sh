A=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab
bash $A/go10.sh > $A/go10.out 2>&1
PREVV=v0.1.0-alpha.119 NEXTV=v0.1.0-alpha.120 bash $A/portalF.sh up v0.1.0-alpha.120 > $A/portalnet-up.out 2>&1
CELL=u119 IP=192.0.3.191 Z=u119.test VER=v0.1.0-alpha.119 OS=ubuntu2404 MEM=4096 SHORT=boston TRUST=1 DNSENGINE=bind bash $A/singleA.sh > $A/singleU.out 2>&1
bash $A/R10.sh > $A/R10.out 2>&1
