# alpha.128 round 2: extra SFTP/FTP logins on Debian 13 (main) and Ubuntu 24.04 (joined) after the update: create,
# confinement smoke from the lab network, remove. Lab only.
export CELL=r9 Z=r9.test MIP=192.0.3.190 BIP=192.0.3.191 IP=192.0.3.190
source /mnt/c/Users/alice/AppData/Local/Temp/claude/b4b12836376297e4/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a128lab/kit/lib.sh
source $A/lib5.sh; source $A/lib6.sh; source $A/lib7.sh
exec > >(tee -a $LOGS/R2sftp-$CELL.log) 2>&1
nup n_sftp.sh >/dev/null
one() { local N=$1 DID=$2 HOST=$3 LOGIN=$4
  echo "== $N: site $DID login $LOGIN $(ts)"
  admn $N GET /api/v1/domains/$DID/access/logins --label logins | sbrief 600
  mkcred $LOGIN
  admn $N POST /api/v1/domains/$DID/access/logins "{\"login\":\"$LOGIN\",\"folder\":\"/public_html\",\"password\":\"$(pw $LOGIN)\"}" --label login-add --timeout 300 | sbrief 700
  gshn $N "id $LOGIN; getent passwd $LOGIN | cut -d: -f3-7; systemctl is-active celikpanel-ftp-$LOGIN.service; findmnt -n /run/celikpanel-ftp/$LOGIN/files | cut -c1-160"
  pw $LOGIN | timeout 200 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_sftp.sh $HOST $LOGIN"
  gshn $N "R=\$(ls -d /var/www/celikpanel/subscriptions/*/sites/$DID/public_html | head -1); ls -ln \$R/lab-up.txt 2>&1; stat -c %u \$R"
  admn $N DELETE /api/v1/domains/$DID/access/logins/$LOGIN --label login-del --timeout 300 | sbrief 500
  gshn $N "id $LOGIN 2>&1 | tail -1; systemctl is-active celikpanel-ftp-$LOGIN.service 2>&1; grep -c $LOGIN /etc/ssh/sshd_config.d/60-celikpanel-file-access.conf 2>/dev/null"
  echo "-- after removal:"; pw $LOGIN | timeout 120 python3 -I $L/lab.py ssh beta1-lab labnet -- "sudo -n bash /root/n_sftp.sh $HOST $LOGIN" | head -6
}
one main $(cat $T/site1.id) $MIP anna-up
one backup $(cat $T/bsite.id) $BIP bert-up
echo "== r2sftp done $(ts)"
