// Small progressive additions; every page reads and works without them.
// - The theme switch: the page follows the system until the visitor chooses.
// - Product screens follow a chosen theme, not only the system's.
// - A language the visitor picks is remembered (boot.js reads it).
// - Copy buttons for commands.
(function () {
  "use strict";
  var root = document.documentElement;
  var media = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  var save = function (key, value) {
    try { window.localStorage.setItem(key, value); } catch (error) { /* a preference only */ }
  };
  var chosenTheme = function () {
    var value = root.getAttribute("data-theme");
    return value === "light" || value === "dark" ? value : null;
  };
  var resolved = function () {
    return chosenTheme() || (media && media.matches ? "dark" : "light");
  };

  var toggle = document.querySelector("[data-theme-toggle]");
  var paint = function () {
    var theme = resolved();
    root.setAttribute("data-resolved", theme);
    if (toggle) {
      toggle.setAttribute("aria-label", toggle.getAttribute(theme === "dark" ? "data-label-light" : "data-label-dark"));
      toggle.setAttribute("aria-pressed", theme === "dark" ? "true" : "false");
    }
    // A <source media="(prefers-color-scheme: dark)"> follows only the
    // system; a chosen theme decides here instead.
    var chosen = chosenTheme();
    document.querySelectorAll("source[data-theme-dark]").forEach(function (source) {
      source.media = chosen ? (chosen === "dark" ? "all" : "not all") : "(prefers-color-scheme: dark)";
    });
  };
  if (toggle) {
    toggle.hidden = false;
    toggle.addEventListener("click", function () {
      var next = resolved() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      save("celikpanel-site-theme", next);
      paint();
    });
  }
  if (media && media.addEventListener) media.addEventListener("change", paint);
  paint();

  document.querySelectorAll("[data-set-lang]").forEach(function (link) {
    link.addEventListener("click", function () { save("celikpanel-language", link.getAttribute("data-set-lang")); });
  });

  document.querySelectorAll("[data-copy]").forEach(function (button) {
    var source = document.getElementById(button.getAttribute("data-copy"));
    if (!source || !navigator.clipboard) return;
    var label = button.querySelector("span");
    var original = label ? label.textContent : "";
    var box = button.closest(".chip, .terminal");
    var status = box && box.querySelector("[data-copy-status]");
    var timer = 0;
    button.hidden = false;
    button.addEventListener("click", function () {
      var done = function (ok) {
        if (label) label.textContent = button.getAttribute(ok ? "data-copied" : "data-failed");
        if (status) status.textContent = button.getAttribute(ok ? "data-copied" : "data-failed");
        button.classList.toggle("done", ok);
        window.clearTimeout(timer);
        timer = window.setTimeout(function () {
          if (label) label.textContent = original;
          button.classList.remove("done");
        }, 2200);
      };
      navigator.clipboard.writeText(source.textContent || "").then(function () { done(true); }, function () { done(false); });
    });
  });

  // The phone menu closes with Escape and when a link in it is followed.
  document.querySelectorAll("details.menu").forEach(function (menu) {
    menu.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && menu.open) {
        menu.open = false;
        menu.querySelector("summary").focus();
      }
    });
  });
})();
