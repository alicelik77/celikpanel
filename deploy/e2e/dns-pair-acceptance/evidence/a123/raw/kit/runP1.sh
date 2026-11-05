A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a123/kit
( CELL=u22 IP=192.0.3.191 Z=u22.test bash $A/R1post.sh > $A/R1postU22.out 2>&1 ) &
( CELL=d12 IP=192.0.3.190 Z=d12.test bash $A/R1post.sh > $A/R1postD12.out 2>&1 ) &
wait; echo "post1 done $(date -u +%T)"
