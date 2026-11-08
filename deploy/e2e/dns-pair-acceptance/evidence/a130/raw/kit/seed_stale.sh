# alpha.130 round: on B (still 129) the Offered tab's untick of "Remote access to databases" as 129 wrote it
# (permission:database_remote in the hidden list). Lab only.
export CELL=s0 Z=s0.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/C3stale-$CELL.log) 2>&1
admn backup PUT /api/v1/offered-versions '{"offering_id":"permission:database_remote","offered":false}' --label offer-stale | sbrief 400
admn backup GET /api/v1/offered-versions --label offered | sbrief 600
gshn backup 'sqlite3 -readonly /var/lib/celikpanel/celikpanel.db "select key,value from panel_settings where key like \"%offer%\"" 2>&1 || ls /var/lib/celikpanel'
