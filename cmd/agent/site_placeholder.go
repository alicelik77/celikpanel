package main

import (
	"fmt"
	"html"
	"strings"
)

// celikPanelSitePlaceholder is the single canonical placeholder contract used
// by both site provisioning and the one-click installer. Exact byte matching
// lets the installer replace only content CelikPanel itself generated.
// celikPanelSitePlaceholder, site oluşturma ve tek tık kurucunun ortak yer
// tutucusudur; bayt bayt eşleşme yalnız CelikPanel'in yazdığını değiştirtir.
func celikPanelSitePlaceholder(domain, projectType string) (string, []byte) {
	name, versions := celikPanelSitePlaceholders(domain, projectType)
	return name, versions[0]
}

// celikPanelSitePlaceholders returns the file name and every placeholder this
// agent recognises as its own, newest first. Sites created by earlier releases
// keep their older page; the installer must still recognise it, or a
// one-click install would refuse those sites as "customer content".
// Önceki sürümlerin yazdığı sayfalar da tanınır; yoksa tek tık kurucu eski
// siteleri "müşteri içeriği" sayıp reddederdi.
func celikPanelSitePlaceholders(domain, projectType string) (string, [][]byte) {
	name := "index.html"
	if projectType == "php" {
		name = "index.php"
	}
	return name, [][]byte{
		sitePlaceholderV2(domain, projectType),
		sitePlaceholderV1(domain, projectType),
	}
}

// sitePlaceholderV2 is the welcome page since v0.1.0-alpha.113. Visitors see
// that the address works; the owner sees what to replace and where. It names
// no paths, versions or modules (no phpinfo), loads nothing external and
// follows the visitor's light or dark preference.
// Ziyaretçi adresin çalıştığını, sahip neyi nereden değiştireceğini görür;
// yol, sürüm, modül yazmaz; dışarıdan hiçbir şey yüklemez.
func sitePlaceholderV2(domain, projectType string) []byte {
	name := html.EscapeString(domain)
	file := "index.html"
	status := ""
	if projectType == "php" {
		file = "index.php"
		// The PHP line proves execution end to end without naming the version.
		status = `
    <p class="status"><span class="dot" aria-hidden="true"></span><?php echo 'PHP çalışıyor · PHP is running'; ?></p>`
	}
	page := `<!doctype html>
<html lang="tr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{NAME}}</title>
<style>
  :root{--bg:#f4f6f9;--card:#ffffff;--ink:#14213d;--muted:#5b6578;--rule:#dfe3ea;--accent:#1d4e89;--ok:#1f7a4d;color-scheme:light}
  @media (prefers-color-scheme:dark){:root{--bg:#0e1626;--card:#152036;--ink:#e8edf5;--muted:#a3adbf;--rule:#26324b;--accent:#8fb6ea;--ok:#6fcf9b;color-scheme:dark}}
  *{box-sizing:border-box}
  body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px 16px;background:var(--bg);color:var(--ink);font:16px/1.55 system-ui,-apple-system,"Segoe UI",Roboto,"Noto Sans",sans-serif}
  main{width:100%;max-width:560px;background:var(--card);border:1px solid var(--rule);border-radius:14px;padding:36px 32px}
  .label{margin:0 0 6px;font-size:13px;letter-spacing:.06em;text-transform:uppercase;color:var(--muted)}
  h1{margin:0 0 12px;font-size:clamp(24px,5vw,32px);line-height:1.2;overflow-wrap:anywhere}
  p{margin:0 0 10px;color:var(--muted)}
  .en{font-size:14px}
  .status{display:flex;align-items:center;gap:8px;margin-top:14px;font-size:14px;color:var(--ok)}
  .dot{width:8px;height:8px;border-radius:50%;background:var(--ok)}
  section{margin-top:24px;padding-top:20px;border-top:1px solid var(--rule)}
  h2{margin:0 0 8px;font-size:15px;color:var(--ink)}
  code{font:13px/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;padding:1px 6px;border:1px solid var(--rule);border-radius:6px;color:var(--ink)}
  footer{margin-top:24px;font-size:13px;color:var(--muted)}
  footer b{color:var(--accent);font-weight:600}
</style>
</head>
<body>
<main>
  <p class="label">Alan adı hazır · <span lang="en">Domain ready</span></p>
  <h1>{{NAME}}</h1>
  <p>Bu adres çalışıyor; sitenin içeriği henüz yüklenmedi.</p>
  <p class="en" lang="en">This address works; the site's content has not been uploaded yet.</p>{{STATUS}}
  <section>
    <h2>Sitenin sahibi misiniz?</h2>
    <p>Panelde bu sitenin <b>Dosyalar</b> bölümünden kendi dosyalarınızı yükleyin ya da bir uygulama kurun. Bu sayfa <code>{{FILE}}</code> dosyasıdır; silin ya da kendi dosyanızla değiştirin.</p>
    <p class="en" lang="en">Upload your files from this site's <b>Files</b> section in the panel, or install an app. This page is the file <code>{{FILE}}</code>; delete it or replace it with your own.</p>
  </section>
  <footer>Barındırma · <span lang="en">Hosting</span>: <b>CelikPanel</b></footer>
</main>
</body>
</html>
`
	page = strings.NewReplacer("{{NAME}}", name, "{{FILE}}", file, "{{STATUS}}", status).Replace(page)
	return []byte(page)
}

// sitePlaceholderV1 is the page written by releases up to v0.1.0-alpha.112.
// Its bytes must never change: installed sites carry exactly this text.
// alpha.112'ye kadar yazılan sayfa; baytları asla değişmemeli.
func sitePlaceholderV1(domain, projectType string) []byte {
	placeholderBody := `  <p>CelikPanel</p>`
	if projectType == "php" {
		placeholderBody = `  <p>CelikPanel · PHP <?php echo htmlspecialchars(PHP_VERSION); ?> · <?php echo date('Y'); ?></p>`
	}
	return []byte(fmt.Sprintf(`<!doctype html>
<html lang="tr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  body{font-family:system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;background:#0f172a;color:#e2e8f0}
  main{text-align:center;padding:2rem}
  h1{font-weight:600}
  p{color:#94a3b8}
</style>
</head>
<body>
<main>
  <h1>%s</h1>
  <p>Bu site hazırlanıyor. / This site is being prepared.</p>
%s
</main>
</body>
</html>
`, domain, domain, placeholderBody))
}
