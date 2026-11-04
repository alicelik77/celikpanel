source /mnt/c/Users/alice/AppData/Local/Temp/claude/c--CELIKBROS-PROJECTS-celikpanel/ab34f56e-94b5-4834-a9ea-4e8dbe057e7f/scratchpad/a118/lab/lib.sh
mkdir -p $B/ev; exec > >(tee -a $B/ev/r3-defect-evidence.txt) 2>&1
echo "== $(ts) PrestaShop docroot (read only)"; GQT=60 gq g_pres.sh
echo "== MediaWiki main page"; lc code - medi.apps118.test "index.php?title=Main_Page"
echo "== phpBB/MediaWiki download User-Agent check from labnet (same NAT)"; nup n403b.sh; nrun n403b.sh
