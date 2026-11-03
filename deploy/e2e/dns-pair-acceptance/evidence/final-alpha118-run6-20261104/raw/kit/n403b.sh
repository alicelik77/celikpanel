u=https://download.phpbb.com/pub/release/3.3/3.3.19/phpBB-3.3.19.tar.bz2
for ua in "CelikPanel/v0.1.0-alpha.118 (+https://celikpanel.net)" "curl/8.14.1" "Go-http-client/1.1"; do echo "UA [$ua]: $(curl -s -o /dev/null -r 0-99 -A "$ua" -w '%{http_code}' $u)"; done
u2=https://releases.wikimedia.org/mediawiki/1.43/mediawiki-1.43.11.tar.gz; echo "wikimedia CelikPanel UA: $(curl -s -o /dev/null -r 0-99 -A 'CelikPanel/v0.1.0-alpha.118 (+https://celikpanel.net)' -w '%{http_code}' $u2)"
