import React from "react";
import ReactDOM from "react-dom/client";
import "./tw.css";
import { AppRuntime } from "./app/AppRuntime";
import { applyTheme, loadTheme } from "./theme";

applyTheme(loadTheme());

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <AppRuntime />
  </React.StrictMode>,
);
