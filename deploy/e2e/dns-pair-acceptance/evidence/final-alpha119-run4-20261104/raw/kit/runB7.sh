A=/mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab
PREVV=v0.1.0-alpha.118 NEXTV=v0.1.0-alpha.119 bash $A/portalF.sh up v0.1.0-alpha.119 > $A/portalnet-up.out 2>&1
CELL=u118 IP=192.0.3.191 Z=u118.test VER=v0.1.0-alpha.118 OS=ubuntu2404 MEM=4096 SHORT=boston TRUST=1 DNSENGINE=bind bash $A/singleA.sh > $A/singleB.out 2>&1
bash $A/B7data.sh > $A/B7data.out 2>&1
bash $A/B7n8n.sh > $A/B7n8n.out 2>&1
SKIPDATA=1 bash $A/Bupd7.sh > $A/Bupd7.out 2>&1
