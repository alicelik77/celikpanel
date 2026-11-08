# guest (root, read-only): PostgreSQL clusters, packages, leftover archives, the files Components edits. Lab only.
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
pg_lsclusters 2>&1 | cat
dpkg -l 'postgresql*' 2>/dev/null | grep -E '^(ii|rc)' | awk '{print $1, $2, $3}'
echo "-- /etc/postgresql:"; ls /etc/postgresql 2>&1 | tr '\n' ' '; echo; echo "-- /var/lib/postgresql:"; ls /var/lib/postgresql 2>&1 | tr '\n' ' '; echo
echo "-- archives:"; ls -la /var/backups/celikpanel/postgresql 2>&1 | tail -n +2
systemctl list-units --all --no-legend --plain 'postgresql*' | cat
