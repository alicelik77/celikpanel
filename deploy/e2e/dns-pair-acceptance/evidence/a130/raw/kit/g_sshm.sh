# guest (root, read-only): sshd Match blocks and includes. Lab only.
cat /etc/bind-peer-inspector/sshd-match.conf; echo ---; grep -n -i "include\|^match" /etc/ssh/sshd_config; ls -la /etc/bind-peer-inspector
