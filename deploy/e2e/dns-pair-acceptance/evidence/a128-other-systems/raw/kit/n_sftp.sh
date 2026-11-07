# labnet (root): SFTP and FTPS smoke for one extra login. usage: n_sftp.sh HOST LOGIN (password on stdin). Lab only.
H=$1; U=$2; IFS= read -r PW; umask 077; printf '%s' "$PW" > /root/sftp-pw
echo "lab upload $(date +%s)" > /root/lab-up.txt
cat > /root/sftp-batch <<'B'
pwd
ls -la /
put /root/lab-up.txt
ls
-cd /..
-ls /..
-get /etc/hostname /root/got-hostname
-cd /proc
B
echo "-- sftp as $U"
timeout 60 sshpass -f /root/sftp-pw sftp -oBatchMode=no -oStrictHostKeyChecking=no -oUserKnownHostsFile=/dev/null -oPubkeyAuthentication=no -oPreferredAuthentications=password -b /root/sftp-batch $U@$H 2>&1 | grep -vE "^Warning: Permanently" | head -30
echo "   got /etc/hostname: $([ -f /root/got-hostname ] && echo YES || echo no)"; rm -f /root/got-hostname
echo "-- ssh shell as $U (must be refused)"; timeout 20 sshpass -f /root/sftp-pw ssh -oStrictHostKeyChecking=no -oUserKnownHostsFile=/dev/null -oPubkeyAuthentication=no -oPreferredAuthentications=password $U@$H id 2>&1 | tail -2
echo "-- ftps as $U"; timeout 40 lftp -u "$U,$PW" -e "set ftp:ssl-force true; set ssl:verify-certificate no; set net:max-retries 1; set net:timeout 10; pwd; ls; put /root/lab-up.txt -o lab-ftps.txt; cd /..; pwd; ls /etc; bye" $H 2>&1 | sed "s,ftp://[^@]*@,ftp://[REDACTED]@," | head -14
rm -f /root/sftp-pw
