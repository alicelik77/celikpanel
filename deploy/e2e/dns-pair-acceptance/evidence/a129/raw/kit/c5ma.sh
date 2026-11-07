# alpha.127 case 5 (mailbox and Access tab password, main, site7.test), EN+TR; the API refuses weak passwords. Lab only.
export CELL=r7 IP=192.0.3.190 Z=r7.test
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a129lab/kit/lib.sh
source $A/lib5.sh
N=main
exec > >(tee -a $LOGS/C5-r7-main.log) 2>&1
python3 $A/uiitems.py pw-mail site7.test $T/pwm.json; python3 $A/uiitems.py pw-access site7.test $T/pwa.json
python3 -c 'import json,sys; json.dump(json.load(open(sys.argv[1]))+json.load(open(sys.argv[2])), open(sys.argv[3],"w"))' $T/pwm.json $T/pwa.json $T/c5ma.json
timeout 2400 python3 -I $H/shots.py r7 main c5ma $T/c5ma.json --langs en,tr --widths desktop > /dev/null
python3 $A/jsum.py c5ma
echo "== API: weak passwords $(ts)"
admn $N POST /api/v1/domains/4/mail/accounts '{"address":"weak@site7.test","password":"[REDACTED]","quota_mb":100}' --label mbox-weak | sbrief 500
admn $N PUT /api/v1/domains/4/access '{"enabled":true,"ssh":false,"password":"[REDACTED]","add_keys":[],"remove_keys":[]}' --label access-weak | sbrief 500
admn $N POST /api/v1/domains/4/access/preview '{"enabled":true,"ssh":false,"password":"[REDACTED]","add_keys":[],"remove_keys":[]}' --label access-weak2 | sbrief 500
admn $N GET /api/v1/domains/4/mail/accounts --label mboxes | sbrief 600
echo "== c5ma done $(ts)"
