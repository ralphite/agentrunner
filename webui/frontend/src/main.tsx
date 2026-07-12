import React from "react";
import ReactDOM from "react-dom/client";
import "./tw.css";
import { App } from "./App";
import { applyTheme, loadTheme } from "./theme";

applyTheme(loadTheme());

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
