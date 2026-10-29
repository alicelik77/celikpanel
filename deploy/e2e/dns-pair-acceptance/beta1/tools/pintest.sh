v=$(dpkg-query -W -f='${Version}' libpcre2-8-0)
printf 'Package: libpcre2-8-0\nPin: version %s\nPin-Priority: 1001\n' "$v" > /etc/apt/preferences.d/beta1-hold-pcre2
apt-get -s install mariadb-server nginx php-fpm 2>&1 | grep -E '^Inst (libpcre2|perl-base)|^E:|broken' | head
rm -f /etc/apt/preferences.d/beta1-hold-pcre2
