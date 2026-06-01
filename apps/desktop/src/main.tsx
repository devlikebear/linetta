import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { App } from "./App";

// Bundled webfonts (offline-first): Newsreader for editorial serif, IBM Plex
// Mono for figures/code. Latin + latin-ext subsets only; Korean falls back to
// the system 명조 stack declared in --font-serif.
import "@fontsource/newsreader/latin-400.css";
import "@fontsource/newsreader/latin-ext-400.css";
import "@fontsource/newsreader/latin-400-italic.css";
import "@fontsource/newsreader/latin-500.css";
import "@fontsource/newsreader/latin-600.css";
import "@fontsource/ibm-plex-mono/latin-400.css";
import "@fontsource/ibm-plex-mono/latin-500.css";

import "./App.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);
