# guest (root): lab load for the large site copy. usage: g_big.sh fill ID DB | sizes ID | dbrows DB. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
R=$(ls -d /var/www/celikpanel/subscriptions/*/sites/$2/public_html 2>/dev/null | head -1)
case $1 in
fill)
  O=$(stat -c %U:%G "$R"); T0=$(date +%s)
  mkdir -p $R/media $R/small
  # many small files: 40 folders x 1000 files of 1-6 KB (PHP/CSS-like text)
  for d in $(seq 1 40); do mkdir -p $R/small/d$d; for f in $(seq 1 1000); do head -c $(( (f % 6 + 1) * 1000 )) /dev/urandom | base64 -w 120 > $R/small/d$d/f$f.txt; done; done
  # large files: 22 x 100 MB of random data (not compressible)
  for i in $(seq 1 22); do head -c 100M /dev/urandom > $R/media/blob$i.bin; done
  echo '<?php echo "big site ", count(glob(__DIR__."/small/*/*")), "\n";' > $R/index.php
  chown -R $O $R
  echo "filled in $(( $(date +%s) - T0 )) s"
  # MariaDB database of a few hundred MB (sequence engine)
  T1=$(date +%s)
  mariadb $3 -e "create table pages (id int primary key auto_increment, slug varchar(64), body varchar(1000)) engine=InnoDB; insert into pages (slug, body) select concat('p', seq), repeat(md5(seq), 31) from seq_1_to_330000; select count(*) from pages;" 2>&1
  echo "database filled in $(( $(date +%s) - T1 )) s"
  ;;
esac
echo "-- $R: $(find $R -type f | wc -l) files, $(du -sh --apparent-size $R | cut -f1) ($(du -sb $R | cut -f1) bytes)"
[ -n "${3:-}" ] && mariadb -N -B -e "select table_schema, round(sum(data_length+index_length)/1048576) as mb from information_schema.tables where table_schema='$3' group by table_schema; select count(*) from $3.pages;" 2>&1
df -h / | tail -1
