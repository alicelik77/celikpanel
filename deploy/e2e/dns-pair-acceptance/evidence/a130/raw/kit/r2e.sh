export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a130-lab/kit/lib.sh
source $A/lib5.sh
exec > >(tee -a $LOGS/R2sftp-$CELL.log) 2>&1
S1=$(cat $T/site1.id); U=cara-up
echo "== SFTP diagnosis on main $(ts)"; mkcred $U
admn main POST /api/v1/domains/$S1/access/logins "{\"login\":\"$U\",\"folder\":\"/public_html\",\"password\":\"$(pw $U)\"}" --label login-add --timeout 300 | sbrief 300
nup n_sftp.sh >/dev/null; pw $U | timeout 200 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_sftp.sh $MIP $U" | head -8
grun main g_sftpdiag.sh $U
admn main DELETE /api/v1/domains/$S1/access/logins/$U --label login-del --timeout 300 | sbrief 120
