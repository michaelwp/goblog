// Builds the bundles the Go server embeds:
//   dist/server.js          — React renderer evaluated by goja on each request
//   dist/assets/app.js/.css — hydrates the public site
//   dist/assets/admin.js/.css — hydrates the admin, including the rich-text editor
import { createRequire } from "node:module";
import path from "node:path";
import * as esbuild from "esbuild";

const outdir = "../backend/internal/web/dist";
const watch = process.argv.includes("--watch");
const prod = !watch;

const shared = {
  bundle: true,
  minify: prod,
  // goja supports ES2017 plus a subset of newer syntax; target it for both bundles.
  target: "es2017",
  jsx: "automatic", // explicit, so files resolved by plugins get it too
  define: { "process.env.NODE_ENV": JSON.stringify(prod ? "production" : "development") },
  logLevel: "info",
};

// renderToString only needs React's synchronous "legacy" server build. The
// default react-dom/server entry also pulls in the streaming renderer, which
// needs MessageChannel and other browser APIs goja doesn't provide.
const require = createRequire(import.meta.url);
const legacyServer = {
  name: "react-dom-server-legacy",
  setup(build) {
    build.onResolve({ filter: /^react-dom\/server$/ }, () => ({
      path: path.join(
        path.dirname(require.resolve("react-dom/package.json")),
        `cjs/react-dom-server-legacy.browser.${prod ? "production" : "development"}.js`,
      ),
    }));
  },
};

// The server renders a plain <textarea> where the rich-text editor goes (the
// browser swaps in the editor after hydration), so the editor library never
// has to run inside goja.
const editorStub = {
  name: "editor-server-stub",
  setup(build) {
    build.onResolve({ filter: /\/RichEditor$/ }, (args) => ({
      path: path.join(args.resolveDir, "RichEditor.server.tsx"),
    }));
  },
};

const builds = [
  { ...shared, entryPoints: ["src/server.tsx"], outfile: `${outdir}/server.js`, format: "iife", platform: "neutral", mainFields: ["browser", "module", "main"], conditions: ["browser"], plugins: [legacyServer, editorStub] },
  { ...shared, entryPoints: { app: "src/client.tsx", admin: "src/admin-client.tsx" }, outdir: `${outdir}/assets`, format: "iife", platform: "browser", sourcemap: !prod },
];

if (watch) {
  for (const b of builds) await (await esbuild.context(b)).watch();
} else {
  await Promise.all(builds.map((b) => esbuild.build(b)));
}
