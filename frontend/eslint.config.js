// ESLint for the React/TypeScript frontend: the recommended JavaScript,
// TypeScript, React and React Hooks rules. Run with `npm run lint` (or
// `make lint` from the repo root).
import js from "@eslint/js";
import react from "eslint-plugin-react";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["node_modules/", ".test-build/"] },
  js.configs.recommended,
  tseslint.configs.recommended,
  react.configs.flat.recommended,
  react.configs.flat["jsx-runtime"], // React 17+ JSX transform: no `import React` needed
  reactHooks.configs.flat["recommended-latest"],
  {
    settings: { react: { version: "detect" } },
    languageOptions: { globals: { ...globals.browser } },
    rules: {
      // Unused variables are errors, except deliberately ignored ones (_name).
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_", varsIgnorePattern: "^_", caughtErrors: "none" }],
    },
  },
  {
    // Build script and tests run in Node.
    files: ["build.mjs", "test/**", "eslint.config.js"],
    languageOptions: { globals: { ...globals.node } },
  },
);
