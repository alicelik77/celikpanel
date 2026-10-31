export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
command -v cwebp || timeout 600 pacman -S --noconfirm --needed libwebp-utils 2>&1 | tail -2
command -v cwebp || exit 1
S=/var/tmp/a114r1/shots; W=/var/tmp/a114r1/shots-webp
rm -rf $W; mkdir -p $W
for f in $S/*.png; do b=$(basename $f .png); r=""; case $b in *-phone) r="-resize 390 0";; esac; cwebp -quiet -q 50 -m 5 $r "$f" -o "$W/$b.webp" 2>/dev/null || { cwebp -quiet -q 50 -resize 0 8000 "$f" -o "$W/$b.webp" 2>/dev/null && echo "height-capped $b"; } || echo "fail $b"; done
cp $S/*.txt $S/shots.jsonl $W/
du -sh $W; ls $W/*.webp | wc -l
E="/mnt/c/CELIKBROS PROJECTS/celikpanel/.claude/worktrees/lab114/deploy/e2e/dns-pair-acceptance/evidence/final-alpha114-run1-20261031"
# the text beside each picture was already scanned clean in the staging; rebuild the tarball from the webp set
python3 -I /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab/sanitize.py $W
cd /var/tmp/a114r1 && rm -rf shots-pack && mkdir shots-pack && cp -r $W shots-pack/shots && (cd shots-pack && tar -czf "$E/final-alpha114-run1-screenshots.tar.gz" shots)
cd "$E" && sha256sum final-alpha114-run1-screenshots.tar.gz > final-alpha114-run1-screenshots.tar.gz.sha256 && cat final-alpha114-run1-screenshots.tar.gz.sha256; ls -la "$E/final-alpha114-run1-screenshots.tar.gz"
