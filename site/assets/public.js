// Small progressive additions; every page reads and works without them.
// - A language the visitor picks is remembered (boot.js reads it).
// - The install command gets a Copy button that copies exactly its text.
(function () {
  "use strict";
  document.querySelectorAll("[data-set-lang]").forEach(function (link) {
    link.addEventListener("click", function () {
      try { window.localStorage.setItem("celikpanel-language", link.getAttribute("data-set-lang")); } catch (error) { /* a preference only */ }
    });
  });

  document.querySelectorAll("[data-copy]").forEach(function (button) {
    var source = document.getElementById(button.getAttribute("data-copy"));
    if (!source || !navigator.clipboard) return;
    var original = button.textContent;
    var box = button.closest(".cmd");
    var status = box && box.querySelector("[data-copy-status]");
    var timer = 0;
    button.hidden = false;
    button.addEventListener("click", function () {
      var done = function (ok) {
        var said = button.getAttribute(ok ? "data-copied" : "data-failed");
        button.textContent = said;
        if (status) status.textContent = said;
        window.clearTimeout(timer);
        timer = window.setTimeout(function () { button.textContent = original; }, 2200);
      };
      navigator.clipboard.writeText(source.textContent || "").then(function () { done(true); }, function () { done(false); });
    });
  });
})();
