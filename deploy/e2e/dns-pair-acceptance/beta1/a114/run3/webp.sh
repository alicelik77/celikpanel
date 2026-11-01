export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
command -v cwebp || timeout 600 pacman -S --noconfirm --needed libwebp-utils 2>&1 | tail -2
command -v cwebp || exit 1
S=/var/tmp/a114r3/shots; W=/var/tmp/a114r3/shots-webp
rm -rf $W; mkdir -p $W
for f in $S/*.png; do b=$(basename $f .png); r=""; case $b in *-phone) r="-resize 390 0";; esac; cwebp -quiet -q 50 -m 5 $r "$f" -o "$W/$b.webp" 2>/dev/null || { cwebp -quiet -q 50 -resize 0 8000 "$f" -o "$W/$b.webp" 2>/dev/null && echo "height-capped $b"; } || echo "fail $b"; done
cp $S/*.txt $S/shots.jsonl $W/
du -sh $W; ls $W/*.webp | wc -l
python3 -I /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/alpha114/lab3/sanitize.py $W
