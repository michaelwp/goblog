// Browser entry for the public site (app.js).
import { hydrateRoot } from "react-dom/client";

import { App } from "./App";
import "./styles.css";
import type { PageData, PublicPageData } from "./types";

declare global {
  interface Window {
    __PAGE__: PageData; // embedded by the Go server
  }
}

hydrateRoot(document.getElementById("root")!, <App {...(window.__PAGE__ as PublicPageData)} />);
