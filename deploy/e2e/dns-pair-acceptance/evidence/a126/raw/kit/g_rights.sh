# guest (root, read-only): saved rights sets and their rows. Lab only.
DB=$(ls /var/lib/celikpanel/*.db 2>/dev/null | head -1); echo "db $DB"
python3 - "$DB" <<'P'
import sqlite3, sys
c = sqlite3.connect("file:%s?mode=ro" % sys.argv[1], uri=True)
for r in c.execute("select s.owner_kind, s.owner_id, s.all_software, count(i.item), group_concat(i.item, ' ') from rights_sets s left join rights_set_items i on i.set_id = s.id and i.item not like 'permission:%' group by s.id"):
    print(" ", r[0], r[1], "all_software", r[2], "software rows", r[3], (r[4] or "")[:500])
P
