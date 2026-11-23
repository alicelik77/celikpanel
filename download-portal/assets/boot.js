// Runs in <head>, before the page is drawn, and keeps the language the
// visitor chose. A visitor who never chose a language and whose browser
// prefers Turkish is taken from an English page to its Turkish one. The page
// follows the system's light or dark setting by itself (public.css). Every
// page works without this file.
(function () {
  var root = document.documentElement;
  var chosen = null;
  try { chosen = window.localStorage.getItem("celikpanel-language"); } catch (error) { chosen = null; }
  var here = root.getAttribute("lang");
  var moved = root.getAttribute("data-page") === "moved";
  var want = chosen === "en" || chosen === "tr" ? chosen : null;
  if (!want && /^tr\b/i.test(navigator.language || "")) want = here === "en" ? "tr" : null;
  if (moved && !want) want = "en";
  if (!want || (want === here && !moved)) return;
  var link = document.querySelector('link[rel="alternate"][hreflang="' + want + '"]');
  if (link && link.href) window.location.replace(link.href + (moved ? "" : window.location.hash));
})();
