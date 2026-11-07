# guest (root, read-only): the lab mail capture = the mailbox $1 on this server. list | token. Lab only.
U=$1; L=${U%@*}; D=${U#*@}
M=$(find / -xdev -type d -path "*$D*$L*" \( -name new -o -name cur \) 2>/dev/null | grep -v "^/proc" | head -4)
case "$2" in
list) echo "maildirs: $M"
  for f in $(ls -t $(for d in $M; do echo $d/*; done) 2>/dev/null | head -${3:-10}); do
    python3 - "$f" <<'P'
import email, sys
from email.header import decode_header, make_header
m = email.message_from_binary_file(open(sys.argv[1], "rb"))
sub = str(make_header(decode_header(m.get("Subject", ""))))
body = ""
for part in m.walk():
    if part.get_content_type() == "text/plain":
        body = part.get_payload(decode=True).decode("utf-8", "replace"); break
import re
body = re.sub(r"#t=\S+", "#t=[REDACTED]", body)
print("--- %s | From: %s | To: %s | Subject: %s" % (m.get("Date"), m.get("From"), m.get("To"), sub))
print("   " + body.strip().replace("\n", "\n   ")[:900])
P
  done ;;
token) for f in $(ls -t $(for d in $M; do echo $d/*; done) 2>/dev/null | head -1); do python3 - "$f" <<'P2'
import email, re, sys
m = email.message_from_binary_file(open(sys.argv[1], "rb"))
for part in m.walk():
    if part.get_content_type() == "text/plain":
        b = part.get_payload(decode=True).decode("utf-8", "replace"); x = re.search(r"verify#t=[REDACTED]\s<>\"]+)", b)
        if x: print(x.group(1)); break
P2
done ;;
esac
