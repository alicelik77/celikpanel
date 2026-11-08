# B lane item 3: the stale hidden entry seeded at 129 is moved as a withdrawn offer on 130. Lab only.
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/b_lib.sh
exec > >(tee -a $LOGS/B3stale-$CELL.log) 2>&1
echo "== $(ts) offered-versions on B (130)"
admn $N GET /api/v1/offered-versions --label offered | sbrief 800
echo "-- audit"; audit 'offering|database.remote|offer_permission' 100
echo "-- database-remote server (offered?)"; admn $N GET /api/v1/database-remote/server --label rs | sbrief 900
echo "== done $(ts)"
