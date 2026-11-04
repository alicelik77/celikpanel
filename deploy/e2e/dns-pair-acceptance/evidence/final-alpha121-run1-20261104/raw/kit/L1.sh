# topology A line 1 (customer A): traffic, disk limit, mail quota. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/L1-$CELL.log) 2>&1
SUBA=$(cat $T/suba); DID=$(cat $T/did); FU=$(cat $T/fu); DR=${DOCDIR:-/}
echo "== L1 start $(ts) subA=$SUBA did=$DID docdir=$DR"
python3 $A/mkup.py $T/big.json big.bin gen:2500000; python3 $A/mkup.py $T/php.json up.php php
for f in big php; do echo "-- upload $f"; as cust118a POST "/api/v1/domains/$DID/files?path=$DR" @$T/$f.json | sbrief 300; done
echo "-- traffic: fetch big.bin 3x from labnet"; lc fetch - c118.test big.bin
echo "-- php upload baseline"; lc phpup - c118.test
echo "-- mail: 950 KB to q@c118.test (quota 1 MB)"; lc smtpsize - q@c118.test 950000
sleep 70; GQT=60 gq gL1.sh $DID q@c118.test
echo "-- mail: second 300 KB to q@c118.test (expect 552 5.2.2)"; lc smtpsize - q@c118.test 300000
echo "-- imap box@c118.test"; lc imap mbx-box box@c118.test
echo "== wait for usage rounds (traffic collection) $(ts)"; waitround; waitround
echo "-- site usage"; as cust118a GET /api/v1/domains/$DID/usage | sbrief 600
echo "-- subscription usage"; subusage cust118a
echo "== CHECK limits: traffic 1 MB, disk 1 MB $(ts)"
setq $SUBA '{"traffic_quota_mb":1,"disk_quota_mb":1}'
waitround
subusage cust118a
echo "-- site (expect traffic-limit page)"; site c118.test
echo "-- imap still works"; lc imap mbx-box box@c118.test
echo "-- new domain (expect QUOTA_DISK_EXCEEDED)"; as cust118a POST /api/v1/domains/create "{\"domain\":\"c118b.test\",\"project_type\":\"php\",\"ssl_type\":\"none\",\"subscription_id\":$SUBA}" | sbrief 400
python3 $A/mkup.py $T/small.json small.txt text:hello
echo "-- file-manager upload (expect refused)"; as cust118a POST "/api/v1/domains/$DID/files?path=$DR" @$T/small.json | sbrief 400
echo "-- SFTP write (expect refused)"; lc sftpput fa118 $FU
GQT=60 gq gL1.sh $DID q@c118.test
echo "== traffic limit lifted, disk still 1 MB $(ts)"
setq $SUBA '{"traffic_quota_mb":0,"disk_quota_mb":1}'
waitround
echo "-- site (expect reopened)"; site c118.test
echo "-- php upload (expect refused while disk full)"; lc phpup - c118.test
echo "== disk limit lifted $(ts)"
setq $SUBA '{"traffic_quota_mb":0,"disk_quota_mb":0}'
waitround
subusage cust118a
echo "-- new domain (expect ok)"; newdomain cust118a $SUBA c118b.test
echo "-- file-manager upload (expect ok)"; as cust118a POST "/api/v1/domains/$DID/files?path=$DR" @$T/small.json | sbrief 400
echo "-- SFTP write (expect ok)"; lc sftpput fa118 $FU
echo "-- php upload (expect ok)"; lc phpup - c118.test
GQT=60 gq gL1.sh $DID q@c118.test
rm -f $T/big.json
echo "== L1 done $(ts)"
