for u in https://releases.wikimedia.org/mediawiki/1.43/mediawiki-1.43.11.tar.gz https://download.phpbb.com/pub/release/3.3/3.3.19/phpBB-3.3.19.tar.bz2; do
  echo "$u curl-UA: $(curl -s -o /dev/null -r 0-99 -w '%{http_code}' $u)  Go-UA: $(curl -s -o /dev/null -r 0-99 -A 'Go-http-client/1.1' -w '%{http_code}' $u)  empty-UA: $(curl -s -o /dev/null -r 0-99 -H 'User-Agent:' -w '%{http_code}' $u)"
done
