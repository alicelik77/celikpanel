# guest (root): can DB user $1 connect? password on stdin. Lab only.
read -r P; MYSQL_PWD="$P" timeout 20 mysql -u "$1" -h 127.0.0.1 -e 'select current_user()' 2>&1 | tail -2; echo "db rc=${PIPESTATUS[0]}"
