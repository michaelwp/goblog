# GoBlog.dev

A place for sharing tech, in English and Indonesian.

GoBlog.dev is a bilingual blog that ships as a single Go binary. That binary serves a server-rendered React site, an admin area for writing and managing articles, and a small JSON API, with everything stored in MongoDB.

- **Backend:** Go and [Fiber](https://gofiber.io) (`backend/`)
- **Frontend:** React 19, rendered on the server inside Go by [goja](https://github.com/dop251/goja) and hydrated in the browser (`frontend/`)
- **Editor:** [TipTap](https://tiptap.dev) visual editor; articles are stored as Markdown
- **Database:** MongoDB for articles, profile, admin account and uploaded images
- **Deployment:** the frontend is compiled into the binary with `embed.FS`, so no Node.js is needed at runtime. It runs locally or in [Apple `container`](https://github.com/apple/container).

## Features

**For readers**

- English and Indonesian editions. `/` redirects by browser language, and every page links to its translation.
- A Wikipedia-inspired layout with a modern look: Contents sidebar, infobox, featured and recent articles, search, and an About page.
- Light, dark or automatic appearance, remembered without a flash on load.
- Fast, crawlable pages: complete HTML from the server, `hreflang` alternates, and a small script (`app.js`) for interactivity.

**For the author** (`/admin`)

- Visual editor with toolbar, keyboard shortcuts, image upload (button, paste or drag), and Markdown and Preview tabs.
- Drafts, publishing, disabling and deleting articles, one at a time or in bulk.
- Safe edits to live articles: changes stay in a draft copy until you click **Publish changes**.
- Autosave every minute, with a warning before leaving unsaved work.
- A language switcher in the editor to move between translations or start a missing one.
- Profile for the About page, with photo upload and a country flag.
- Password-protected, with a one-time setup code, strong-password rules and rate-limited logins.

## Quick start

Requires macOS with [Apple `container`](https://github.com/apple/container) for the container workflow. Running the app directly on your Mac also needs Go 1.26+ and Node 20+.

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
| `make run` | Build the frontend and run the Go server on your Mac |
| `make watch` | Rebuild frontend bundles on change (restart `make run` to pick them up) |
| `make build` | Compile `bin/blog` with the frontend embedded |
| `make test` | Type-check and test the frontend, then vet and test the backend |
| `make image` / `make clean` | Build only the container image / remove build output |

## Configuration

`.env` in the repo root holds the connection settings. `make run` and `make up` load it automatically, and it's gitignored; `.env.example` is the template.

| Variable | Purpose |
| --- | --- |
| `MONGODB_URI` | MongoDB used by `make run` (default: the `make mongo` container on `localhost:27018`). `make up` always uses its bundled MongoDB container. |
| `MONGODB_DB` | Database name (default `blog`). |
| `PORT` | HTTP port (default `8080`). |
| `LANGUAGES` | Supported languages, default first (default `en,id`). Keep in sync with `frontend/src/lib/i18n.ts`. |

No secrets go in `.env`: the admin password is stored, hashed, in MongoDB.

## Using the admin

Sign in at `/admin`. The top bar has **Articles**, **Profile**, **Password**, **View site** and **Log out**.

### Articles

The list groups each article with its translations. Each language chip shows its state, and **+ EN** / **+ ID** starts a missing translation. Filter by **All / Published / Drafts / Disabled**.

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
| `/{lang}` | Main page: welcome, featured and recent articles |
| `/{lang}/posts/{slug}` | Article, with contents, infobox and `hreflang` alternates |
| `/{lang}/search?q=` | Search titles, summaries and bodies of published articles |
| `/{lang}/about` | About the blog owner |
| `/media/{id}.{ext}` | Uploaded images |
| `/admin` | Admin area |
| `/assets/*` | Embedded JS and CSS, cached for a year and versioned by content hash |
| `/api/v1/languages` | Supported languages (JSON) |
| `/api/v1/{lang}/posts` | Published posts in a language (JSON) |
| `/api/v1/{lang}/posts/{slug}` | One published post plus its available languages (JSON) |
| `/healthz` | Liveness check |

## How it works

### Serving a page

1. Fiber handles e.g. `GET /id/posts/hello-world` and loads the article from MongoDB.
2. It passes the page data to `render()` in the embedded React server bundle, which runs in a pooled goja runtime.
3. The HTML goes into a page shell with `window.__PAGE__` (the same data as JSON) and a script: `app.js` for public pages, or `admin.js` for the admin, which includes the editor. Readers never download the editor.
4. In the browser, the script hydrates the server-rendered markup using that JSON.

### Project layout

```text
backend/
  cmd/server/          main: config, MongoDB, startup, `reset-admin` command
  internal/api/        JSON API and the Fiber app
  internal/web/        pages, admin, autosave, bulk actions, media and about handlers
  internal/ssr/        goja renderer pool
  internal/posts/      articles: MongoDB and in-memory stores, statuses, draft copies
  internal/profile/    About-page profile store
  internal/auth/       admin credential (bcrypt), password rules
  internal/media/      image uploads (GridFS)
  internal/web/dist/   frontend build output, embedded into the binary
frontend/
  src/App.tsx          public pages;  src/pages/About.tsx
  src/admin/           admin screens, visual editor, autosave, Markdown serializer
  src/components/      layout, Markdown renderer, contents
  src/lib/             i18n dictionaries, article parser, countries, date formatting
  src/server.tsx       server entry (run by goja)
  src/client.tsx       app.js entry;  src/admin-client.tsx  admin.js entry
  test/                frontend tests (node:test)
```

MongoDB collections: `posts` (one document per translation, unique on slug and language), `profile`, `admin`, and the `media` GridFS bucket (`media.files`, `media.chunks`).

### SSR constraints

The server bundle runs in goja, which has the ECMAScript standard library but no DOM, Node or `Intl` APIs. Frontend code must therefore:

- render identically in goja and the browser: no `window` or `Date.now()` during render; browser-only work goes in `useEffect`
- avoid `Intl` during render: dates are formatted by `lib/format.ts` with the i18n dictionaries, and times in the admin are formatted after hydration
- keep browser-only libraries out of the server bundle: the visual editor is replaced by a plain textarea there (`admin/RichEditor.server.tsx`, swapped in by `build.mjs`)

Pages are full server renders and links are plain `<a>` tags; there is no client-side router.

## Testing

`make test` runs:

- **Frontend** (`npm test` in `frontend/`): the Markdown renderer and the editor's serializer, including tricky cases like text that looks like Markdown, links with parentheses, and combined bold/italic.
- **Backend** (`go test ./...`): stores, API, SSR, pages, admin, autosave, bulk actions, uploads and security checks, using in-memory stores.

Set `MONGODB_TEST_URI=mongodb://localhost:27018` to also run the MongoDB integration tests. Each creates a temporary database and drops it afterwards.

## Design

The site keeps Wikipedia's structure (Contents sidebar, infobox, language menu, featured and recent sections) with a simple, modern look: a narrow reading column, serif headings, hairline dividers and one accent color. On narrower screens the infobox moves above the text, and on phones the contents list moves into the article. The **Appearance** menu switches between Automatic, Light and Dark, stored in a `theme` cookie that the server reads so the right theme renders from the first paint.

## Troubleshooting

- **`make up` prints a `192.168.x.x` URL instead of `localhost:8080`:** macOS is blocking the container port forwarder. Allow `container` under **System Settings → Privacy & Security → Local Network**, then run `make up` again.
- **Containers can't reach each other by name:** Apple `container` needs an admin-configured DNS domain for that, so `make up` passes the app MongoDB's IP address instead. If MongoDB restarts on its own, run `make up` again.
- **Frontend changes don't show up:** the bundles are embedded when Go compiles. Run `make frontend` (or keep `make watch` running) and restart `make run`, or run `make up` for the containers.
