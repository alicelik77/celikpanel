# labnet (root): one SSH password sign-in attempt (password on stdin, never printed). usage: n_pwssh.sh HOST USER. Lab only.
read -r SSHPASS; export SSHPASS
timeout 20 sshpass -e ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PreferredAuthentications=password,keyboard-interactive -o PubkeyAuthentication=no -o NumberOfPasswordPrompts=1 "$2@$1" 'echo PASSWORD-LOGIN-OK' 2>&1 | grep -vi "warning: permanently" | tail -2
echo "rc ${PIPESTATUS[0]}"
