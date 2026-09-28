package posts

import "time"

// SeedPosts is sample content so the site renders something out of the box.
// Bodies are plain text: blank lines separate paragraphs, and a paragraph
// starting with "## " is a section heading.
func SeedPosts() []Post {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 9, 0, 0, 0, time.UTC) }
	return []Post{
		{
			Slug: "hello-world", Lang: "en", PublishedAt: day(20),
			Title:   "Hello, world",
			Summary: "The first post on this blog, and a short tour of how it is built.",
			Body: `Welcome! This is the first post on this blog. Every page you read here is rendered on the server by React and delivered as complete HTML.

## How it works

The site is a single Go program built with the Fiber framework. When a page is requested, Go loads the post from MongoDB and runs React inside an embedded JavaScript engine to produce the HTML.

The browser then loads a small script that attaches React to the page, so it stays interactive without rendering everything a second time.

## Languages

Posts can be written in English and Indonesian. When a translation exists, the language menu next to the title links to it.`,
		},
		{
			Slug: "hello-world", Lang: "id", PublishedAt: day(20),
			Title:   "Halo, dunia",
			Summary: "Tulisan pertama di blog ini, sekaligus tur singkat tentang cara blog ini dibuat.",
			Body: `Selamat datang! Ini adalah tulisan pertama di blog ini. Setiap halaman yang Anda baca di sini dirender di server oleh React dan dikirim sebagai HTML lengkap.

## Cara kerjanya

Situs ini adalah satu program Go yang dibangun dengan framework Fiber. Saat sebuah halaman diminta, Go mengambil tulisan dari MongoDB lalu menjalankan React di dalam mesin JavaScript tertanam untuk menghasilkan HTML.

Browser kemudian memuat skrip kecil yang menghubungkan React ke halaman, sehingga halaman tetap interaktif tanpa merender semuanya dua kali.

## Bahasa

Tulisan dapat ditulis dalam bahasa Inggris dan bahasa Indonesia. Jika terjemahan tersedia, menu bahasa di samping judul akan menautkannya.`,
		},
		{
			Slug: "why-ssr", Lang: "en", PublishedAt: day(25),
			Title:   "Why server-side rendering",
			Summary: "Faster first paint and pages search engines can read.",
			Body: `Server-side rendering (SSR) means building a page's HTML on the server for each request, instead of sending an empty page and building it in the browser with JavaScript.

## Performance

Because the browser receives complete HTML, it can show content as soon as the response arrives. Readers on slow devices or networks see the article before any JavaScript has run.

## Search engines

Crawlers read the same HTML a visitor sees. Titles, descriptions and links to translations are all present in the first response, which makes pages easier to index correctly.

## Trade-offs

Rendering on the server costs CPU time for every request, and code must avoid browser-only APIs while rendering. For a blog, where most visits are reads, the trade is usually worth it.`,
		},
	}
}
