// Runs in <head>, before the page is drawn: applies a theme the visitor chose
// (otherwise the page follows the system) and keeps the language the visitor
// chose. A visitor who never chose a language and whose browser prefers
// Turkish is taken from an English page to its Turkish one. Every page works
// without this file.
(function () {
  var root = document.documentElement;
  var stored = function (key) {
    try { return window.localStorage.getItem(key); } catch (error) { return null; }
  };
  var theme = stored("celikpanel-site-theme");
  if (theme === "light" || theme === "dark") root.setAttribute("data-theme", theme);
  var dark = theme === "dark" || (theme !== "light" && window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
  root.setAttribute("data-resolved", dark ? "dark" : "light");

  var chosen = stored("celikpanel-language");
  var here = root.getAttribute("lang");
  var moved = root.getAttribute("data-page") === "moved";
  var want = chosen === "en" || chosen === "tr" ? chosen : null;
  if (!want && /^tr\b/i.test(navigator.language || "")) want = here === "en" ? "tr" : null;
  if (moved && !want) want = "en";
  if (!want || (want === here && !moved)) return;
  var link = document.querySelector('link[rel="alternate"][hreflang="' + want + '"]');
  if (link && link.href) window.location.replace(link.href + (moved ? "" : window.location.hash));
})();
