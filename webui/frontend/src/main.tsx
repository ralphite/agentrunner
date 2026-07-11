import React from "react";
import ReactDOM from "react-dom/client";
// Tailwind entry (theme tokens + utilities) loads first so utility classes sit
// in the lowest-priority @layer; the hand-written base/slice CSS below is
// unlayered and therefore still wins during the incremental migration.
import "./tw.css";
// base stylesheet must load BEFORE App (whose component modules import their
// slice stylesheets: styles.conv/composer/panel/nav/rs.css) so slice rules of
// equal specificity override the base by source order.
import "./styles.css";
import { App } from "./App";
import { applyTheme, loadTheme } from "./theme";

applyTheme(loadTheme());

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
