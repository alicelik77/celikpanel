# guest (root, read-only): what apt would take for postgresql-18 and from where (simulation only). Lab only.
dpkg -l postgresql-common postgresql-client-common libpq5 2>/dev/null | grep -E '^(ii|rc)' | awk '{print "installed", $2, $3}'
apt-cache policy postgresql-common postgresql-client-common libpq5 postgresql-18 2>/dev/null | grep -E '^[a-z]|Installed|Candidate|[0-9]+ http|^ +\*\*\*|^ +[0-9.~a-z+-]+ [0-9]+$'
echo "-- simulate"
apt-get -s -o Debug::NoLocking=1 install --no-install-recommends postgresql-18 2>/dev/null | grep -E '^Inst' 
