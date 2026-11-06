A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a125acc/kit
net() { dmesg | grep -q "sub channel open failed" && { echo "STOP: host network adapter failed again (dmesg) $(date -u +%T)"; exit 2; }; }
net; TAG=fix HOLD=20 bash $A/repro1.sh > $A/C1fix.out 2>&1
grep -q '"version": "v0.1.0-alpha.125"' $A/C1fix.out || { echo "STOP: main did not reach 125 $(date -u +%T)"; exit 1; }
net; bash $A/post3.sh > $A/post3.out 2>&1
net; bash $A/post4.sh > $A/post4.out 2>&1
net; bash $A/case2.sh > $A/case2.out 2>&1
echo "runC2 done $(date -u +%T)"
