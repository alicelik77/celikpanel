A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit
bash $A/build131.sh > $A/build131.out 2>&1
sed -i 's#bash $A/portalF.sh add v0.1.0-alpha.131 | tail -1##' $A/build131.sh
bash $A/r2g.sh > $A/r2g.out 2>&1
echo chainG done
