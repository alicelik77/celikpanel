# labnet (root): SFTP backup target user rbk (key only) and the decrypt step. Lab only.
# usage: nrb.sh setup | key | hostfp | list | decrypt OBJECT (passphrase on stdin)
case "$1" in
setup)
  id rbk >/dev/null 2>&1 || useradd -m -s /bin/bash rbk
  mkdir -p /home/rbk/backups /home/rbk/.ssh; [ -f /root/rbkkey ] || ssh-keygen -q -t ed25519 -N '' -C rbk-lab -f /root/rbkkey
  cp /root/rbkkey.pub /home/rbk/.ssh/authorized_keys; chown -R rbk:rbk /home/rbk; chmod 700 /home/rbk/.ssh; chmod 600 /home/rbk/.ssh/authorized_keys
  command -v gpg >/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -q gnupg >/dev/null 2>&1
  echo "rbk ready; gpg $(gpg --version | head -1)" ;;
key) cat /root/rbkkey ;;
hostfp) for k in /etc/ssh/ssh_host_*_key.pub; do ssh-keygen -lf $k; done ;;
list) find /home/rbk/backups -type f -printf '%P %s\n' | sort ;;
decrypt)
  read -r P; f=/home/rbk/backups/$2; w=$(mktemp -d); printf '%s' "$P" > $w/pass.txt; chmod 600 $w/pass.txt
  echo "## sidecar"; python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(json.dumps({k:v for k,v in d.items() if "pass" not in k.lower()})[:900])' "$f.json" 2>&1
  gpg --batch --pinentry-mode loopback --passphrase-file $w/pass.txt --output $w/x.cpbak --decrypt "$f" 2>&1 | tail -4; echo "gpg rc=${PIPESTATUS[0]}"
  sha256sum $w/x.cpbak "$f" | sed "s#$w/##"; tar -tzf $w/x.cpbak 2>&1 | head -8
  rm -rf $w ;;
esac
