#!/bin/bash
# Build the lab download portal (stand-in for https://celikpanel.net) for one
# acceptance dist, the way deploy/test-fresh-install-entry-boundary-contract.sh
# builds its signed test portal: a lab Ed25519 key, a signed release-manifest-v2
# and the release's own download-portal/get.sh with ONLY its pinned public-key
# digest replaced by the lab key's. The lab private key stays on the lab host.
# usage: portal.sh COMMIT  -> /var/tmp/a117r1/portal/site. Lab only.
set -euo pipefail
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
C=$1
B=/var/tmp/a127r1
TOOLS=$B/tools-$C
D=/var/tmp/cp-pair-accept/dist/$C-acceptance-license
version=$(sed -n 's/^bootstrap_release_version=//p' $TOOLS/download-portal/get.sh)
sequence=$(sed -n 's/^bootstrap_release_sequence=//p' $TOOLS/download-portal/get.sh)
# lab difference (alpha.118 run 1): the candidate's get.sh still names alpha.117 (the release commit bumps it);
# PORTAL_VERSION/PORTAL_SEQUENCE set the release the lab portal advertises and get.sh's bootstrap lines.
version=${PORTAL_VERSION:-$version}; sequence=${PORTAL_SEQUENCE:-$sequence}
archive_src=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["archive"])' $D/dist.json)
K=$B/portal-key
if [ ! -f $K/private.pem ]; then
  mkdir -p $K; chmod 700 $K
  openssl genpkey -algorithm ED25519 -out $K/private.pem 2>/dev/null; chmod 600 $K/private.pem
fi
openssl pkey -in $K/private.pem -pubout -out $K/public.pem 2>/dev/null
public_sha=$(sha256sum $K/public.pem | awk '{print $1}')
P=$B/portal-$version; S=$P/site
rm -rf $P; mkdir -p $S/releases/$version/linux/amd64
R=$S/releases/$version/linux/amd64
name=celikpanel-$version-linux-amd64.tar.gz
cp $archive_src $R/$name
# the archive root must be celikpanel-$version (get.sh checks it)
[ "$(tar -tzf $R/$name | sed -n 1p)" = "celikpanel-$version/" ] || { echo "archive root is not celikpanel-$version/"; tar -tzf $R/$name | head -2; exit 1; }
commit=$(tar -xzOf $R/$name celikpanel-$version/release.commit | tr -d '\n')
[ "$commit" = "$C" ] || { echo "release.commit $commit != $C"; exit 1; }
sha=$(sha256sum $R/$name | awk '{print $1}'); size=$(stat -Lc %s $R/$name)
printf '%s  %s\n' "$sha" "$name" > $R/$name.sha256
cat > $R/release-manifest-v2 <<M
format=celikpanel-release-manifest-v2
sequence=$sequence
version=$version
commit=$commit
published_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
os=linux
arch=amd64
archive=$name
archive_sha256=$sha
archive_size=$size
M
openssl pkeyutl -sign -rawin -inkey $K/private.pem -in $R/release-manifest-v2 -out $R/release-manifest-v2.sig
[ "$(stat -Lc %s $R/release-manifest-v2.sig)" = 64 ]
cp $K/public.pem $S/release-signing-ed25519.pem
echo "$version" > $S/releases/latest.txt
cp $TOOLS/download-portal/get.sh $S/get.sh
sed -i "s|^bootstrap_release_public_key_sha256=.*|bootstrap_release_public_key_sha256=$public_sha|; s|^bootstrap_release_version=.*|bootstrap_release_version=$version|; s|^bootstrap_release_sequence=.*|bootstrap_release_sequence=$sequence|" $S/get.sh
grep -Fxq "bootstrap_release_public_key_sha256=$public_sha" $S/get.sh
diff $TOOLS/download-portal/get.sh $S/get.sh > $P/get.sh.diff || true
echo "== get.sh difference from the release's download-portal/get.sh:"; cat $P/get.sh.diff
echo "$version $(echo $(wc -c < $S/get.sh))" > /dev/null
cp $(dirname $0)/fakecurl.sh $P/curl; chmod 755 $P/curl
cat $R/release-manifest-v2
find $S -type f -printf '%P %s\n'
