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
// that the address works and nothing is published yet; the owner sees, in the
// panel's own words, where to put the site. It tells visitors nothing about
// the server: no software (not even that PHP runs), paths or versions; it runs
// no script, loads nothing external and follows the visitor's light or dark
// preference. The PHP page is plain HTML in index.php. Only PHP sites have
// the panel's Applications section, so only their page mentions it.
// Ziyaretçi adresin çalıştığını, sahip panelde neyi nereden yapacağını görür;
// sunucudaki yazılımı (PHP dahil), yolu, sürümü söylemez; betik çalıştırmaz.
// Uygulamalar bölümü yalnız PHP sitelerinde olduğundan yalnız onlarda geçer.
func sitePlaceholderV2(domain, projectType string) []byte {
	name := html.EscapeString(domain)
	file := "index.html"
	stepTR := `Panelde bu sitenin <b>Dosyalar</b> bölümünden sitenizin dosyalarını yükleyin.`
	stepEN := `Upload your site's files from this site's <b>Files</b> section in the panel.`
	if projectType == "php" {
		file = "index.php"
		stepTR = `Panelde bu sitenin <b>Dosyalar</b> bölümünden sitenizin dosyalarını yükleyin ya da <b>Uygulamalar</b> bölümünden WordPress gibi hazır bir uygulama kurun.`
		stepEN = `Upload your site's files from this site's <b>Files</b> section in the panel, or install a ready-made application such as WordPress from its <b>Applications</b> section.`
	}
	page := `<!doctype html>
<html lang="tr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{NAME}}</title>
<style>
  :root{--bg:#eef1f5;--card:#ffffff;--ink:#16213a;--muted:#5a6478;--soft:#f5f7fa;--rule:#dde2ea;--accent:#1f5fa8;color-scheme:light}
  @media (prefers-color-scheme:dark){:root{--bg:#0d1422;--card:#141e31;--ink:#e6ebf3;--muted:#9aa5b8;--soft:#1a263c;--rule:#26334b;--accent:#86b2ea;color-scheme:dark}}
  *{box-sizing:border-box}
  html,body{height:100%}
  body{margin:0;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:16px;min-height:100vh;padding:32px 16px;background:var(--bg);color:var(--ink);font:16px/1.6 system-ui,-apple-system,"Segoe UI",Roboto,"Noto Sans",sans-serif;-webkit-font-smoothing:antialiased}
  main{width:100%;max-width:600px;background:var(--card);border:1px solid var(--rule);border-top:4px solid var(--accent);border-radius:12px;padding:40px 36px 32px;box-shadow:0 1px 2px rgba(16,24,40,.05)}
  .label{margin:0 0 10px;font-size:12px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:var(--accent)}
  h1{margin:0 0 16px;font-size:clamp(26px,6vw,36px);font-weight:700;line-height:1.15;letter-spacing:-.01em;overflow-wrap:anywhere}
  p{margin:0}
  .lead{font-size:17px;color:var(--ink)}
  .en{margin-top:4px;font-size:14px;color:var(--muted)}
  section{margin-top:28px;padding:20px 22px;background:var(--soft);border:1px solid var(--rule);border-radius:10px}
  h2{margin:0 0 12px;font-size:15px;font-weight:600}
  h2 span{font-weight:400;color:var(--muted)}
  ol{margin:0;padding-left:20px;display:grid;gap:12px}
  li{padding-left:2px;color:var(--ink);font-size:15px}
  li .en{font-size:13px}
  b{font-weight:600}
  code{font:13px/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;padding:1px 6px;background:var(--card);border:1px solid var(--rule);border-radius:5px}
  footer{font-size:13px;color:var(--muted)}
  @media (max-width:480px){main{padding:28px 20px 24px}section{padding:16px}}
</style>
</head>
<body>
<main>
  <p class="label">Alan adı hazır · <span lang="en">Domain ready</span></p>
  <h1>{{NAME}}</h1>
  <p class="lead">Bu adres çalışıyor. Site henüz yayımlanmadı.</p>
  <p class="en" lang="en">This address works. The site has not been published yet.</p>
  <section>
    <h2>Sitenin sahibi misiniz? <span lang="en">· Own this site?</span></h2>
    <ol>
      <li>{{STEP_TR}}<p class="en" lang="en">{{STEP_EN}}</p></li>
      <li>Bu sayfa <code>{{FILE}}</code> dosyasıdır. Kendi siteniz hazır olunca bu dosyayı silin ya da üzerine yazın.<p class="en" lang="en">This page is the file <code>{{FILE}}</code>. When your own site is ready, delete it or overwrite it.</p></li>
    </ol>
  </section>
</main>
<footer>CelikPanel ile barındırılıyor · <span lang="en">Hosted with CelikPanel</span></footer>
</body>
</html>
`
	page = strings.NewReplacer("{{NAME}}", name, "{{FILE}}", file, "{{STEP_TR}}", stepTR, "{{STEP_EN}}", stepEN).Replace(page)
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
