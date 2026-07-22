(function () {
  try {
    var theme = localStorage.getItem("arwebui.theme");
    if (theme === "light" || theme === "dark") {
      document.documentElement.setAttribute("data-theme", theme);
    }
    var dark = theme === "dark" ||
      (theme !== "light" && window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches);
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", dark ? "#0f0f11" : "#ffffff");
  } catch (_) {
    // Storage can be unavailable in hardened/private browser contexts. CSS
    // still follows prefers-color-scheme in that case.
  }
})();
