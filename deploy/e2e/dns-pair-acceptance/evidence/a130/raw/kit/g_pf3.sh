journalctl --since 11:43:40 --until 11:45:00 -o cat --no-pager | grep -E "635F142F2B|celikpanel-relay|wrapper|TLS" | head -8 | cut -c1-330
