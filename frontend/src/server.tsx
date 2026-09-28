// Entry point evaluated by goja inside the Go server. It has no DOM and no
// Node APIs; it exposes a single global the server calls per request.
import { renderToString } from "react-dom/server";

import { AdminApp, adminHead } from "./admin/AdminApp";
import { App, head } from "./App";
import { isAdminPage, type PageData } from "./types";

declare global {
  // eslint-disable-next-line no-var
  var render: (pageJSON: string) => string;
}

globalThis.render = (pageJSON) => {
  const props = JSON.parse(pageJSON) as PageData;
  const html = isAdminPage(props) ? renderToString(<AdminApp {...props} />) : renderToString(<App {...props} />);
  return JSON.stringify({ html, ...(isAdminPage(props) ? adminHead(props) : head(props)) });
};
