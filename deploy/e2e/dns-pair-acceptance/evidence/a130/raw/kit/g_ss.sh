ss -ltnp | grep -E ":1923 |:443 |:80 " | sed 's/  */ /g'
