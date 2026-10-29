import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./style.css";
try {
  document.documentElement.dataset.theme =
    localStorage.getItem("malta-theme") === "light" ? "light" : "dark";
} catch {
  document.documentElement.dataset.theme = "dark";
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
