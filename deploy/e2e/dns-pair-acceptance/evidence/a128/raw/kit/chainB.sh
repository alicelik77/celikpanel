export CELL=r8 Z=r8.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
A=/mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit
( source $A/lib.sh; source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh; echo "== install PostgreSQL on the joined server $(ts)"; installc backup postgresql; gshn backup 'pg_lsclusters | cat' ) > $A/c5pg-install.out 2>&1
PGP=$(grep -oE "^1[0-9] +main +[0-9]{4}" $A/c5pg-install.out | awk '{print $3}' | head -1)
bash $A/c5db.sh backup bsite.id postgresql ${PGP:-5432} > $A/c5db-backup.out 2>&1
echo chainB done
