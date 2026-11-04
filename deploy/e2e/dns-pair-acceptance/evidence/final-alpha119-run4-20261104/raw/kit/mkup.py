# mkup.py OUT NAME (gen:BYTES | php | eicar | text:STRING) -> file-manager upload body. Lab only.
import base64, json, os, sys
out, name, spec = sys.argv[1:4]
if spec.startswith("gen:"): data = os.urandom(int(spec[4:]))
elif spec == "php": data = b"<?php header('Content-Type: text/plain'); echo 'file_uploads=' . var_export((bool)ini_get('file_uploads'), true) . ' files=' . count($_FILES) . ' errors=' . json_encode(array_map(function($f){return $f['error'];}, $_FILES)) . \"\n\";\n"
elif spec == "eicar": data = b"X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"
elif spec.startswith("text:"): data = spec[5:].encode()
json.dump({"action": "upload", "file_name": name, "content": base64.b64encode(data).decode()}, open(out, "w"))
