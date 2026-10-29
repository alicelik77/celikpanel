# beta1 lab only: hold libpcre2-8-0 at its installed version (apt preferences), usage: holdpcre.sh on|off
if [ "$1" = on ]; then v=$(dpkg-query -W -f='${Version}' libpcre2-8-0); printf 'Package: libpcre2-8-0\nPin: version %s\nPin-Priority: 1001\n' "$v" > /etc/apt/preferences.d/beta1-hold-pcre2; else rm -f /etc/apt/preferences.d/beta1-hold-pcre2; fi
ls /etc/apt/preferences.d/
