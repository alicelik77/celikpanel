source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab/env.sh
for c in a4-c:main a4-c:backup a4-d:main a4-d:backup; do bash $H/collnode.sh ${c%%:*} ${c##*:} > /dev/null 2>&1; echo "collected $c"; done
bash $H/portalnet.sh down 2>&1 | tail -25 > /var/tmp/a114r1/logs/portalnet-requests.log; tail -3 /var/tmp/a114r1/logs/portalnet-requests.log
for c in a4-c:main a4-c:backup a4-d:main a4-d:backup; do lab stop ${c%%:*} ${c##*:} | tr -d '\n '; echo; done
for z in fa114.test archa114.test shopb114.test shop114.test gone114.test custb114.test cust114.test ui114.test wiz114.test c114.test d114.test; do infra undelegate $z > /dev/null 2>&1 && echo "undelegated $z"; done
pgrep -a qemu | cut -c1-80
free -m | head -2
