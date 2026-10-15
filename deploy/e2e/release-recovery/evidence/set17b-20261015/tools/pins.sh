#!/bin/bash
# set17b: the release pins inside every archive of both documents, read from the tarballs (reads only).
source <redacted>
for d in update second; do
  ART=$(cat $R/artifacts-$d.path)
  for role in baseline good defective; do
    a=$(python3 -I -c 'import json,sys;print(json.load(open(sys.argv[1]))[sys.argv[2]]["archive"])' "$ART" $role)
    echo "### $d $role $(basename "$a") sha256=$(sha256sum "$a" | cut -c1-64)"
    top=$(tar -tzf "$a" | head -n 1 | cut -d/ -f1)
    for f in deploy/release-sequence-policy libexec/get.sh download-portal/get.sh; do
      if tar -tzf "$a" "$top/$f" >/dev/null 2>&1; then
        echo "== $top/$f sha256=$(tar -xOzf "$a" "$top/$f" | sha256sum | cut -c1-64)"
        case $f in
          deploy/release-sequence-policy) tar -xOzf "$a" "$top/$f" ;;
          *get.sh) tar -xOzf "$a" "$top/$f" | grep -n '^bootstrap_release_' ;;
        esac
      else echo "== $top/$f absent"; fi
    done
  done
done
