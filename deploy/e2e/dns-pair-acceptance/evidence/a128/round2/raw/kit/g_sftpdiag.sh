U=$1
echo "shadow: $(getent shadow $U | cut -d: -f2 | cut -c1-4)... (len $(getent shadow $U | cut -d: -f2 | wc -c))"; passwd -S $U 2>&1
sshd -T -C user=$U,host=labnet,addr=192.0.3.1 2>&1 | grep -iE "^(passwordauthentication|kbdinteractiveauthentication|authenticationmethods|usepam|chrootdirectory|forcecommand|allowusers|denyusers|allowgroups|denygroups) " 
grep -n "" /etc/ssh/sshd_config.d/60-celikpanel-file-access.conf | head -30
ls -ld /run/celikpanel-ftp/$U /run/celikpanel-ftp/$U/files
journalctl -u ssh -u sshd --since -3min -o cat --no-pager | grep -iE "$U|pam|fail|denied|invalid" | tail -8 | cut -c1-220
grep -rhE "^\s*(PasswordAuthentication|KbdInteractiveAuthentication|UsePAM|AuthenticationMethods)" /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null
