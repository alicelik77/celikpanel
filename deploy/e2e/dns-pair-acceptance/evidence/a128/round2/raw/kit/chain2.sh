A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit
for c in c2php c3web c4mail c6api; do bash $A/$c.sh > $A/$c.out 2>&1; done
bash $A/c5db.sh main site1.id mysql > $A/c5db-main.out 2>&1
for c in c5b c3node c2ver; do bash $A/$c.sh > $A/$c.out 2>&1; done
echo chain2 done
