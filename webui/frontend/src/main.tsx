import React from "react";
import ReactDOM from "react-dom/client";
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
