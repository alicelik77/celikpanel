# usage: wizdata.py DIR KIND ZONE MAINIP SHORT [MAIN_SITE]   -> wizard step patches (as web/src sends them)
# KIND: pair-main | pair-backup-mail | pair-backup-web | single
import json, sys
w, kind, z, ip, short = sys.argv[1:6]
main_site = sys.argv[6] if len(sys.argv) > 6 else "here"
mail = ["nginx", "php-fpm", "mariadb", "postfix", "dovecot", "roundcube", "rspamd"]
web = ["nginx", "php-fpm", "mariadb"]
if kind in ("pair-main", "single", "pair-main-web"):
    comps = web if kind == "pair-main-web" else mail
    steps = {"domain": {"main_domain": z}, "count": {"server_count": 1 if kind == "single" else 2}, "name": {"short_name": short},
             "roles": {"purpose": "custom", "dns_mode": "local", "main_site": main_site, "customization": {"components": comps}},
             "names": {"panel_domain": f"{short}.{z}", "mail_hostname": f"mail.{z}", "ns1": f"ns1.{z}", "ns2": f"ns2.{z}",
                       "main_dns_server": "this", "local_ip": ip},
             "dnshome": {"dns_mode": "local", "dns_role": "primary", "dns_engine": "pdns", "main_dns_server": "this"},
             "mail": {"mail_dns": {"mx": "now", "dmarc": "add"}}}
    if main_site == "none":
        steps["roles"].pop("main_site")
    if kind != "single":
        steps["names"]["peer_ns"] = f"ns2.{z}"
    if kind == "pair-main-web":
        steps["names"].pop("mail_hostname")
        steps.pop("mail")
else:
    comps = mail if kind == "pair-backup-mail" else web
    steps = {"roles": {"purpose": "custom", "customization": {"components": comps}}, "name": {"short_name": short},
             "names": {"panel_domain": f"{short}.{z}"}, "dnshome": {"dns_engine": "pdns"}}
    if kind == "pair-backup-mail":
        steps["names"]["mail_hostname"] = f"mail.{z}"          # the first server's name: must be refused
        steps["names2"] = {"mail_hostname": f"mail.{short}.{z}"}  # the proposal: mail.<panel name>
for k, v in steps.items():
    json.dump(v, open(f"{w}/{k}.json", "w"))
print("wizard data", kind, sorted(steps))
