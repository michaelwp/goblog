# GoBlog.dev

[![CI](https://github.com/michaelwp/goblog/actions/workflows/ci.yml/badge.svg)](https://github.com/michaelwp/goblog/actions/workflows/ci.yml)

A place for sharing tech, in English and Indonesian.

GoBlog.dev is a bilingual blog that ships as a single Go binary. That binary serves a server-rendered React site, an admin area for writing and managing articles, and a small JSON API, with everything stored in MongoDB.

- **Backend:** Go and [Fiber](https://gofiber.io) (`backend/`)
- **Frontend:** React 19, rendered on the server inside Go by [goja](https://github.com/dop251/goja) and hydrated in the browser (`frontend/`)
- **Editor:** [TipTap](https://tiptap.dev) visual editor; articles are stored as Markdown
- **Database:** MongoDB for articles, profile, admin account and uploaded images
- **Deployment:** the frontend is compiled into the binary with `embed.FS`, so no Node.js is needed at runtime. It runs locally, in [Apple `container`](https://github.com/apple/container), or on [Fly.io](https://fly.io).

## Features

**For readers**

- English and Indonesian editions. `/` redirects by browser language, and every page links to its translation.
- A Wikipedia-inspired layout with a modern look: Contents sidebar, infobox, featured article, articles grouped by category, and an About page.
- Categories and tags: every category and tag has its own page, and search can combine words, a category and a tag.
- Sharing: round logo buttons above and below each article (X, Facebook, LinkedIn, WhatsApp, Telegram, email, copy link, and the phone's share sheet), and pages carry Open Graph tags so shared links show a title, summary and the article's first image. No third-party scripts.
- Light, dark or automatic appearance, remembered without a flash on load.
- Fast, crawlable pages: complete HTML from the server, `hreflang` alternates, and a small script (`app.js`) for interactivity.

**For the author** (`/admin`)

- Visual editor with toolbar, keyboard shortcuts, image upload (button, paste or drag), and Markdown and Preview tabs.
- Drafts, publishing, disabling and deleting articles, one at a time or in bulk.
- Safe edits to live articles: changes stay in a draft copy until you click **Publish changes**.
- Autosave every minute, with a warning before leaving unsaved work.
- A language switcher in the editor to move between translations or start a missing one.
- Categories (managed in the admin, named in both languages) and free-form tags.
- Profile for the About page, with photo upload and a country flag.
- Password-protected, with a one-time setup code, strong-password rules and rate-limited logins.

**For developers**

- One command each to run, test, lint and deploy (`make`).
- Unit tests for both halves (`go test`, `node:test`), plus MongoDB integration tests.
- Standard lint rules for Go (golangci-lint) and React/TypeScript (ESLint), enforced by a git pre-commit hook.
- GitHub Actions CI on every push and pull request, publishing a container image to GitHub Container Registry.

## Quick start

Requires macOS with [Apple `container`](https://github.com/apple/container) for the container workflow. Running the app directly on your Mac, testing and linting also need Go 1.26+ and Node 20+.

**Everything in containers:**

```sh
make up          # build the image, start MongoDB + the app, print the URL
make admin-code  # show the one-time code for first-time admin setup
```

**App on your Mac, MongoDB in a container:**

```sh
make mongo       # MongoDB in a container, published on localhost:27018
make run         # build the frontend and start the server on :8080
```

Then open http://localhost:8080. An empty database is seeded with sample articles on first start.

**Setting up the admin account** (once):

1. Open http://localhost:8080/admin. With no admin account yet, it shows the setup page.
2. Enter the setup code: run `make admin-code`, or read the terminal running `make run`. The code expires after an hour; reopening `/admin/setup` prints a new one.
3. Choose a password: at least 12 characters with an uppercase letter, a lowercase letter, a number and a symbol. A strength meter shows the rules as you type.

## Commands

Run `make` to list every target.

| Command | What it does |
| --- | --- |
| `make up` | Build the image and run the app and MongoDB in containers |
| `make down` | Stop and remove both containers (the MongoDB data volume is kept) |
| `make logs` / `make ps` | Follow the app's logs / list this project's containers |
| `make admin-code` | Show the admin setup code from the app container's log |
| `make admin-reset` | Forgot the password? Remove it so `/admin` offers setup again |
| `make destroy` | `down`, then delete the MongoDB volume (**all data**), the network and the image |
| `make mongo` | Start only MongoDB in a container, on `localhost:27018` |
| `make db-setup` | Once, on a new database: create the collections with schema validators, and their indexes, in `MONGODB_URI`. Safe to rerun. |
| `make run` | Build the frontend and run the Go server on your Mac |
| `make watch` | Rebuild frontend bundles on change (restart `make run` to pick them up) |
| `make build` | Compile `bin/blog` with the frontend embedded |
| `make test` / `make test-unit` | Unit tests: frontend (`node:test`) and backend (`go test`, in-memory stores) |
| `make test-integration` | Integration tests against a real MongoDB (`make mongo` first, or set `MONGODB_TEST_URI`) |
| `make test-all` | Unit and integration tests |
| `make cover` | Backend test coverage |
| `make lint` | golangci-lint for Go; ESLint and a type check for the frontend |
| `make fmt` | Fix formatting and auto-fixable lint issues |
| `make check` | Lint and unit tests: what the pre-commit hook runs |
| `make hooks` | Install the git pre-commit hook |
| `make image` / `make clean` | Build only the container image / remove build output |
| `make fly-setup` | Once: create the Fly.io app and send it `MONGODB_URI` and `MONGODB_DB` from `.env` |
| `make fly-deploy` | Build on Fly's builders and deploy to Fly.io (one machine) |
| `make fly-secrets` | Resend the MongoDB settings from `.env` to Fly (restarts the app) |
| `make fly-token` | Create a Fly deploy token and save it as the `FLY_API_TOKEN` GitHub secret |
| `make fly-status` / `make fly-logs` | The Fly app's machines / follow its logs |
| `make fly-admin-code` / `make fly-admin-reset` | `admin-code` / `admin-reset`, for the app on Fly |

## Configuration

`.env` in the repo root holds the connection settings. `make run` and `make up` load it automatically, and it's gitignored; `.env.example` is the template.

| Variable | Purpose |
| --- | --- |
| `MONGODB_URI` | MongoDB used by `make run` and `make up` (default: the `make mongo` container on `localhost:27018`, which `make up` starts when needed). Any URI works, e.g. Atlas. |
| `MONGODB_DB` | Database name (default `blog`). |
| `PORT` | HTTP port (default `8080`). |
| `LANGUAGES` | Supported languages, default first (default `en,id`). Keep in sync with `frontend/src/lib/i18n.ts`. |
| `SITE_URL` | Public address used in share links and link previews, e.g. `https://goblog.dev` (default: the address each request came in on, which is right unless several domains point at the app). |
| `MONGODB_TEST_URI` | MongoDB for `make test-integration` (default: `localhost:27018`, the `make mongo` container). Tests create and drop their own temporary databases. |

No secrets go in `.env`: the admin password is stored, hashed, in MongoDB.

## Using the admin

Sign in at `/admin`. The top bar has **Articles**, **Categories**, **Profile**, **Password**, **View site** and **Log out**.

### Articles

The list groups each article with its translations and groups articles by category, with "No category" last. Each language chip shows its state, and **+ EN** / **+ ID** starts a missing translation. Filter by status (**All / Published / Drafts / Disabled**) and by category.

Each translation has a status:

| Status | Visible to readers? | Editor buttons |
| --- | --- | --- |
| **Draft** | No. It can be saved unfinished (no body yet). | Save draft · Publish |
| **Published** | Yes | Save draft · Publish changes · Disable · Move to drafts |
| **Disabled** | No: hidden from pages, search, language links and the API, but not deleted | Save changes · Publish again |

The buttons appear both above and below the form. Pressing Enter always takes the safe action (save, never publish).

- **Editing a live article never changes it directly.** Edits, including autosaves, go into a *draft copy*. Readers, search and the API keep seeing the published version, while the editor and **Preview** show the draft. **Publish changes** replaces the live version with the draft, and **Discard changes** drops it. The list marks such translations **Edited**. Disabling or moving an article to drafts folds pending changes in first.
- **Autosave:** changes are saved about once a minute, and a status next to the buttons shows "Unsaved changes", "Saving…" or "Draft saved at …". Autosave never publishes. A new article is created as a draft once it has a slug and title. Clicking a button while an autosave is in progress waits for it to finish, and leaving the page with unsaved changes asks first.
- **Switching languages:** the **Language** dropdown lists every language with its state (e.g. "Bahasa Indonesia · Draft"). Choosing one saves your work, then opens that translation, or starts it with the same slug and date if it doesn't exist yet.
- **Preview** opens any article, including drafts, exactly as the site will show it, with a banner saying it isn't public.
- **Several articles at once:** tick articles (or **Select all**) and use the bar that appears to **Publish**, **Disable**, **Move to drafts** or **Delete**. Each row's **⋯** menu does the same for one article. These actions cover every translation of an article. Publishing skips empty drafts and says how many, and Disable and Delete ask for confirmation.
- **Deleting:** remove one translation, or the whole article in every language, from the editor's danger zone or the list.
- **The slug can't be changed** after an article is created: it's the article's address and what links its translations together.

### Categories and tags

Both describe the whole article: they're shared by all its translations, so setting them on one language sets them on the others. They apply as soon as you save, even on a published article; only the text goes through the draft copy.

- **Categories** are managed under **Admin → Categories**. Each has a name in every language (e.g. Security / Keamanan) and a slug used in its address (`/en/categories/security`); leave the slug empty to generate it from the English name. Categories are unique: a second category can't reuse a slug, or a name in either language (ignoring case). Names can be changed later but slugs can't. Deleting a category keeps its articles, which become uncategorized. In the editor, pick at most one category per article.
- **Tags** are keywords typed in the editor's **Tags** field. Press Enter or comma after each; suggestions come from tags already in use. Tags are normalized (lowercase, spaces become hyphens) and can use letters, numbers and `- + # .` (so `c++`, `c#` and `node.js` work). Up to 10 per article.

On the site, the main page groups articles by category, each category and tag has its own page, and **Search** can combine words, a category and a tag. Only published articles count.

### Writing

The editor is visual: text looks as it will on the site while you write. The toolbar covers text style (paragraph, heading, subheading), bold, italic, strikethrough, inline code, links, bulleted and numbered lists, quotes, code blocks, dividers, images, and undo/redo. Shortcuts: ⌘/Ctrl + B, I, K (link) and Z. Selecting an image shows a caption field. Two more tabs: **Markdown** shows and edits the underlying text, and **Preview** renders it exactly as the site will.

Articles and profile bios are stored as Markdown:

| Markdown | Result |
| --- | --- |
| `**bold**`, `*italic*`, `~~strike~~`, `` `code` `` | Font styling |
| `[text](https://…)` | Link (external links open in a new tab) |
| `## Heading`, `### Subheading` | Sections; `##` headings are listed under Contents |
| `> quote`, `- item`, `1. item` | Quote and lists |
| ```` ``` ```` fenced block | Code block |
| `![caption](/media/….png)` | Image; on its own line it becomes a figure with a caption |
| `---` | Divider |

**Images:** click **Image**, or paste or drag images into the editor. Uploads (PNG, JPEG, GIF or WebP, up to 5 MB) are stored in MongoDB and served from `/media/…` with a one-year cache. Files are checked by their content, so an SVG or HTML file renamed to `.png` is rejected. Links to `https://` images also work.

Limits: no underline (Markdown can't store it), no nested lists (nested items are kept as normal items), and no line breaks inside a paragraph (Enter starts a new one). Opening and saving an article without editing it leaves its Markdown unchanged.

### Profile and password

- **Profile** fills the About page. Name, photo, location, country, email and links are shared between languages; the headline and bio are written per language. If one language has no text, the page shows the other with a note.
  - **Photo:** click **Upload photo**. Share-page links (Google Drive, Google Photos, Dropbox) are refused, because they open a web page rather than the image.
  - **Country:** shown as a flag emoji before the location. Windows has no flag emoji and shows the two-letter code instead.
  - **Links:** one per line as `Label | https://…`, up to 10.
- **Password:** changing it signs out every other session. If you forget it, run `make admin-reset` (or `cd backend && go run ./cmd/server reset-admin` when running locally), then set a new one at `/admin`.

### Security

- The password is stored as a bcrypt hash in the `admin` collection, next to the key that signs session cookies.
- Sessions last 12 hours in an HttpOnly, `SameSite=Strict` cookie. Changing the password invalidates all other sessions.
- Setup needs a one-time code from the server log, so whoever reaches a fresh site first can't claim it.
- Login and setup allow 5 failed attempts per minute per IP, and form posts from other sites are rejected.
- Admin pages are sent with `no-store`, `noindex` and `X-Frame-Options: DENY`.
- Article text is rendered as React elements, never as raw HTML. Links must be `https://`, `http://`, `mailto:`, `/…` or `#…`, and images `https://` or `/media/…`; anything else (e.g. `javascript:`) shows as plain text.
- Drafts, disabled articles and pending changes never reach readers: they're excluded from public pages, search, the page's embedded data and the API.

## Routes

| Route | Response |
| --- | --- |
| `/` | Redirect to the preferred language |
| `/{lang}` | Main page: welcome, featured article, and articles grouped by category |
| `/{lang}/posts/{slug}` | Article, with contents, infobox and `hreflang` alternates |
| `/{lang}/search?q=&category=&tag=` | Search published articles by words, category and tag, in any combination |
| `/{lang}/categories/{slug}` | Articles in a category |
| `/{lang}/tags/{tag}` | Articles with a tag |
| `/{lang}/about` | About the blog owner |
| `/media/{id}.{ext}` | Uploaded images |
| `/admin` | Admin area (articles, categories, profile, password) |
| `/assets/*` | Embedded JS and CSS, cached for a year and versioned by content hash |
| `/api/v1/languages` | Supported languages (JSON) |
| `/api/v1/{lang}/posts?tag=&category=` | Published posts in a language, optionally filtered (JSON) |
| `/api/v1/{lang}/posts/{slug}` | One published post plus its available languages (JSON) |
| `/api/v1/{lang}/categories` | Categories with their names and post counts (JSON) |
| `/api/v1/{lang}/tags` | Tags in use, most used first (JSON) |
| `/healthz` | Liveness check |

## How it works

### Serving a page

1. Fiber handles e.g. `GET /id/posts/hello-world` and loads the article from MongoDB.
2. It passes the page data to `render()` in the embedded React server bundle, which runs in a pooled goja runtime.
3. The HTML goes into a page shell with `window.__PAGE__` (the same data as JSON) and a script: `app.js` for public pages, or `admin.js` for the admin, which includes the editor. Readers never download the editor.
4. In the browser, the script hydrates the server-rendered markup using that JSON.

### Project layout

```text
Makefile               every command (run `make` for the list)
Dockerfile             container image: frontend build → Go build → distroless
.githooks/pre-commit   runs `make check` before each commit (`make hooks` installs it)
.github/workflows/     CI/CD: lint, tests, container image (published to ghcr.io)
backend/
  .golangci.yml        Go lint rules
  cmd/server/          main: config, MongoDB, startup, `reset-admin` command
  internal/api/        JSON API and the Fiber app
  internal/web/        pages, admin, autosave, bulk actions, media and about handlers
  internal/ssr/        goja renderer pool
  internal/posts/      articles: MongoDB and in-memory stores, statuses, draft copies
  internal/categories/ categories store (unique slugs and names)
  internal/profile/    About-page profile store
  internal/auth/       admin credential (bcrypt), password rules
  internal/media/      image uploads (GridFS)
  internal/web/dist/   frontend build output, embedded into the binary
frontend/
  build.mjs            esbuild: server.js, app.js and admin.js into backend/internal/web/dist
  eslint.config.js     frontend lint rules
  src/App.tsx          public pages;  src/pages/About.tsx
  src/admin/           admin screens, visual editor, autosave, Markdown serializer
  src/components/      layout, Markdown renderer, contents
  src/lib/             i18n dictionaries, article parser, countries, dates, query strings, useHydrated
  src/server.tsx       server entry (run by goja)
  src/client.tsx       app.js entry;  src/admin-client.tsx  admin.js entry
  test/                frontend tests (node:test)
```

MongoDB collections: `posts` (one document per translation, unique on slug and language; category and tags are kept in sync across an article's translations), `categories` (unique slug, and unique names per language ignoring case), `profile`, `admin`, and the `media` GridFS bucket (`media.files`, `media.chunks`).

### SSR constraints

The server bundle runs in goja, which has the ECMAScript standard library but no DOM, Node or `Intl` APIs. Frontend code must therefore:

- render identically in goja and the browser: no `window` or `Date.now()` during render; browser-only work goes in event handlers or `useEffect`, and UI that exists only in the browser (the visual editor, tag chips, local times) switches in with `useHydrated()` (`lib/useHydrated.ts`)
- use only the ECMAScript standard library while rendering: browser APIs such as `URLSearchParams` don't exist in goja (build query strings with `lib/query.ts`)
- avoid `Intl` during render: dates are formatted by `lib/format.ts` with the i18n dictionaries, and times in the admin are formatted after hydration
- keep browser-only libraries out of the server bundle: the visual editor is replaced by a plain textarea there (`admin/RichEditor.server.tsx`, swapped in by `build.mjs`)

Pages are full server renders and links are plain `<a>` tags; there is no client-side router.

## Testing

- **`make test-unit`** runs quickly, with no database:
  - **Frontend** (`npm test` in `frontend/`, Node's built-in test runner): the Markdown renderer and editor serializer (e.g. text that looks like Markdown, links with parentheses, combined bold/italic), the article parser, dates and dictionaries, countries and flags, tag normalization (checked against the same cases as the Go version) and the password strength meter.
  - **Backend** (`go test ./...`): stores, the API, server-side rendering, public and admin pages, autosave, bulk actions, categories and tags, uploads, security checks and helper functions, using in-memory stores.
- **`make test-integration`** runs the MongoDB store tests against a real database: the `make mongo` container by default, or `MONGODB_TEST_URI=mongodb://…`. Each test creates a temporary database and drops it afterwards.

## Development workflow

1. After cloning, run `make hooks` once to install the pre-commit hook.
2. While working, `make run` serves the app (with `make mongo` for the database) and `make watch` rebuilds the frontend.
3. Before committing, the hook runs `make check` (lint and unit tests) automatically. Run `make fmt` to fix formatting, and `make test-integration` when you change MongoDB code.
4. `make up` rebuilds and restarts the containers with your changes.

## CI/CD

GitHub Actions (`.github/workflows/ci.yml`) runs on every push to `main`, every pull request and every version tag:

| Job | What it runs |
| --- | --- |
| **Lint** | golangci-lint (same pinned version as `make lint`), then `make lint-frontend` |
| **Unit tests** | `make test-unit` |
| **Integration tests** | `make test-integration` against a MongoDB 8 service container |
| **Container image** | Builds the Dockerfile once the three jobs above pass |
| **Deploy to Fly.io** | Pushes to `main` only: deploys that image to Fly.io |

On pushes to `main` and on version tags, the image is also published to GitHub Container Registry as `ghcr.io/michaelwp/goblog`, tagged:
- `latest` and `main` for `main`
- `1.2.3` and `1.2` for tag `v1.2.3`
- `sha-<commit>` for every build

Pull requests only build the image. A newer push to the same pull request cancels the older run.

- **Releasing a version:** `git tag v1.0.0 && git push origin v1.0.0`.
- **Image visibility:** the first publish creates a *private* package. To let a server pull it without logging in, make it public under the repository's **Packages → goblog → Package settings**.
- **Deploying:** every push to `main` that passes the checks is deployed to Fly.io (`fly.toml`). The image job also pushes the image to Fly's registry, and the deploy job runs `flyctl deploy` with that exact image, so Fly doesn't rebuild it. This needs the `FLY_API_TOKEN` repository secret (`make fly-token`). To deploy by hand, run `make fly-deploy`.

### First deploy to Fly.io

1. Install flyctl and log in: `brew install flyctl && fly auth login`. Add a card at fly.io/trial, or trial machines stop after 5 minutes.
2. Point `MONGODB_URI` in `.env` at a hosted cluster (e.g. MongoDB Atlas), and allow `0.0.0.0/0` in Atlas → Network Access (Fly has no fixed outgoing IP).
3. `make fly-setup`, then `make fly-deploy`. The site is at `https://<app>.fly.dev`.
4. Open `/admin` and get the setup code with `make fly-admin-code`.
5. `make fly-token` so CI can deploy pushes to `main`.

Don't also turn on deploys from Fly's GitHub integration in the dashboard, or every push deploys twice.

## Code quality

- **Go:** [golangci-lint](https://golangci-lint.run) v2 with its standard linters (errcheck, govet, ineffassign, staticcheck, unused), plus gofmt and goimports (`backend/.golangci.yml`). `make lint` runs a pinned version, built with the project's Go on first use.
- **Frontend:** ESLint with the recommended JavaScript, [typescript-eslint](https://typescript-eslint.io), React and React Hooks rules (`frontend/eslint.config.js`), plus `tsc --noEmit`.
- **Pre-commit hook:** run `make hooks` once per clone. Before every commit, `make check` runs lint and the unit tests, and the commit is stopped if anything fails (full output in `.git/pre-commit.log`). It checks the working tree, including unstaged changes. Skip it once with `git commit --no-verify`.

## Design

The site keeps Wikipedia's structure (Contents sidebar, infobox, language menu, a featured article and articles grouped by category) with a simple, modern look: a narrow reading column, serif headings, hairline dividers and one accent color. On narrower screens the infobox moves above the text, and on phones the contents list moves into the article. The **Appearance** menu switches between Automatic, Light and Dark, stored in a `theme` cookie that the server reads so the right theme renders from the first paint.

## Troubleshooting

- **`make up` prints a `192.168.x.x` URL instead of `localhost:8080`:** macOS is blocking the container port forwarder. Allow `container` under **System Settings → Privacy & Security → Local Network**, then run `make up` again.
- **Containers can't reach each other by name:** Apple `container` needs an admin-configured DNS domain for that, so `make up` passes the app MongoDB's IP address instead. If MongoDB restarts on its own, run `make up` again.
- **Frontend changes don't show up:** the bundles are embedded when Go compiles. Run `make frontend` (or keep `make watch` running) and restart `make run`, or run `make up` for the containers.
- **A commit is refused:** the pre-commit hook found a lint error or failing test; the output is in `.git/pre-commit.log`, and `make check` reproduces it. `make fmt` fixes formatting. In an emergency, `git commit --no-verify` skips the hook once.
- **The first `make lint` is slow:** it builds the pinned golangci-lint version once (about a minute); later runs take seconds.
