source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
exec > >(tee -a $LOGS/R7cert-$CELL.log) 2>&1
echo "== guide chunks $(ts)"; GQT=60 gq g_guide7.sh
echo "== portal guide pages in the built portal tree (source + manifest check) $(ts)"
R=$B/repo-v0.1.0-alpha.119; G=$(mktemp -d); git -c safe.directory='*' -C $R archive $C118 download-portal/guide | tar -x -C $G
echo "  pages: $(find $G/download-portal/guide -name '*.html' | wc -l) (en $(ls $G/download-portal/guide/en | wc -l), tr $(ls $G/download-portal/guide/tr | wc -l))"; (cd $G/download-portal/guide && sha256sum -c manifest.sha256 2>&1 | tail -2; echo "  manifest lines $(wc -l < manifest.sha256)")
grep -n "guide" $R/deploy/build-download-portal.sh | head -5; rm -rf $G
echo "== certificates $(ts)"
for spec in cust119a:prog119.test cust119b:py119.test cust119b:flask119.test cust119b:fast119.test cust119n:n8n119.test; do IFS=: read cu dn <<< "$spec"
  AS_TIMEOUT=300 as $cu POST /api/v1/domains/$(cat $T/id-$dn)/ssl/letsencrypt '{"auto_renew":true}' | sbrief 200
  echo "  $dn certificate: $(as $cu GET /api/v1/domains/$(cat $T/id-$dn)/runtime-apps | jget certificate | cut -c1-200)"; done
