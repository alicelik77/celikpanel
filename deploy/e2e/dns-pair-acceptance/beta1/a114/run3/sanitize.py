# sanitize.py DIR : key-based redaction of password-like values in every text file (lab evidence staging)
import pathlib, re, sys
rx = re.compile(r'''(?P<k>['"](?:FTPPassword|password|new_password|secret|recovery_codes|pending_token)['"][ ]*:[ ]*)(?P<q>['"])(?!\[REDACTED)[^'"]*(?P=q)''')
n = 0
for p in pathlib.Path(sys.argv[1]).rglob("*"):
    if p.is_file() and p.suffix != ".png" and p.stat().st_size < 50_000_000:
        t = p.read_text(errors="replace")
        u = rx.sub(lambda m: m.group('k') + m.group('q') + '[REDACTED]' + m.group('q'), t)
        if u != t:
            p.write_text(u); n += 1
print("sanitized files:", n)
