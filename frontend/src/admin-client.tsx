// Browser entry for the admin (admin.js). Kept separate from app.js so blog
// readers never download the editor.
import { hydrateRoot } from "react-dom/client";

import { AdminApp } from "./admin/AdminApp";
import "./styles.css";
import type { AdminPageData } from "./types";

hydrateRoot(document.getElementById("root")!, <AdminApp {...(window.__PAGE__ as AdminPageData)} />);
