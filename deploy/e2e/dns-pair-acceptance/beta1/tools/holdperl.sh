# beta1 lab only: hold or release Ubuntu's perl point updates (apt preferences), usage: holdperl.sh on|off|test
if [ "$1" = on ] || [ "$1" = test ]; then
  : > /etc/apt/preferences.d/beta1-hold-perl
  for p in perl-base perl libperl5.38t64 perl-modules-5.38; do
    v=$(dpkg-query -W -f='${Version}' $p 2>/dev/null); [ -z "$v" ] && v=$(apt-cache policy $p | awk '/Installed:/{print $2}')
    if [ -n "$v" ] && [ "$v" != "(none)" ]; then printf 'Package: %s\nPin: version %s\nPin-Priority: 1001\n\n' $p "$v" >> /etc/apt/preferences.d/beta1-hold-perl; else
      v=$(dpkg-query -W -f='${Version}' perl-base); printf 'Package: %s\nPin: version %s\nPin-Priority: 1001\n\n' $p "$v" >> /etc/apt/preferences.d/beta1-hold-perl; fi
  done
  apt-get -s install mariadb-server nginx php-fpm 2>&1 | grep -E '^Inst (perl|libperl)|^E:|broken' | head -5; echo "sim-rc=${PIPESTATUS[0]}"
fi
[ "$1" = off ] || [ "$1" = test ] && rm -f /etc/apt/preferences.d/beta1-hold-perl
ls /etc/apt/preferences.d/
