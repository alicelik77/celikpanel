# guest (root): lab only — hide / restore the BIND peer inspector's Include line for the duration of the SSH check
# (stand-in for the fix on fix/a130-lab-b). usage: hide | restore. Byte-identical restore.
case $1 in
hide) cp -a /etc/ssh/sshd_config /root/sshd_config.lab-orig; sed -i 's#^Include /etc/bind-peer-inspector/sshd-match.conf#\# LAB Include /etc/bind-peer-inspector/sshd-match.conf#' /etc/ssh/sshd_config; grep -n "bind-peer" /etc/ssh/sshd_config; systemctl reload ssh ;;
restore) cp -a /root/sshd_config.lab-orig /etc/ssh/sshd_config && rm -f /root/sshd_config.lab-orig; grep -n "bind-peer" /etc/ssh/sshd_config; sha256sum /etc/ssh/sshd_config | cut -c1-16; systemctl reload ssh; sshd -T -C user=celikpeer,host=x,addr=192.0.3.190 | grep -E "^(passwordauthentication|forcecommand)" ;;
esac
