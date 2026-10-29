# beta1 lab only: stage repo.powerdns.com as the pdns-server candidate, read the wizard, then remove it again
set -u
if [ "$1" = on ]; then
  install -d /etc/apt/keyrings
  curl -fsS --max-time 30 -o /etc/apt/keyrings/auth-49-pub.asc https://repo.powerdns.com/FD380FBB-pub.asc || { echo KEY-FETCH-FAILED; exit 0; }
  echo "deb [signed-by=/etc/apt/keyrings/auth-49-pub.asc] http://repo.powerdns.com/debian trixie-auth-49 main" > /etc/apt/sources.list.d/beta1-pdns.list
  printf 'Package: pdns-*\nPin: origin repo.powerdns.com\nPin-Priority: 600\n' > /etc/apt/preferences.d/beta1-pdns
else
  rm -f /etc/apt/sources.list.d/beta1-pdns.list /etc/apt/preferences.d/beta1-pdns /etc/apt/keyrings/auth-49-pub.asc
fi
apt-get update -qq 2>&1 | tail -3
apt-cache policy pdns-server | head -8
