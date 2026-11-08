A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit
for c in d12f u22f arf; do bash $A/rmcell.sh $c; done
grep -o "\"restart_required\": [a-z]*" /var/tmp/a130o/logs/install-arf-main.log | head -2
