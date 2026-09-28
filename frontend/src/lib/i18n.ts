// Keep in sync with the backend's LANGUAGES env var.
export const locales = ["en", "id"] as const;
export const defaultLocale: Locale = "en";

export type Locale = (typeof locales)[number];

export const hasLocale = (value: string): value is Locale =>
  (locales as readonly string[]).includes(value);

// Strings may contain {placeholders}; fill them with format().
const dictionaries = {
  en: {
    siteTitle: "GoBlog.dev",
    siteTagline: "Sharing tech",
    siteDescription: "A place for sharing tech, in English and Indonesian.",
    fromSite: "From GoBlog.dev, a place for sharing tech",
    languageName: "English",

    categories: "Categories",
    category: "Category",
    tags: "Tags",
    otherArticles: "Other articles",
    viewAll: "View all {n} →",
    categoryTitle: "{name}",
    categoryIntro: "Articles in the {name} category.",
    tagTitle: "Tagged “{tag}”",
    tagIntro: "Articles tagged {tag}.",
    allCategories: "All categories",
    allTags: "All tags",
    searchText: "Words to find",
    applyFilters: "Show articles",
    clearFilters: "Clear filters",
    popularTags: "Browse by tag",
    browseByCategory: "Browse by category",
    filteredCount: "{n} articles",
    filteredCountOne: "{n} article",
    noMatches: "No articles match these filters.",
    jumpToContent: "Jump to content",
    appearance: "Appearance",
    color: "Color",
    themeAuto: "Automatic",
    themeLight: "Light",
    themeDark: "Dark",
    mainMenu: "Main menu",
    mainPage: "Main page",
    about: "About",
    aboutTitle: "About me",
    infoLocation: "Location",
    infoEmail: "Email",
    infoLinks: "Links",
    profileEmpty: "The blog owner hasn't written a profile yet.",
    profileFallback: "This profile hasn't been translated into English yet.",
    contents: "Contents",
    top: "(Top)",
    article: "Article",
    read: "Read",
    languagesOne: "{n} language",
    languagesMany: "{n} languages",

    welcome: "Welcome to {site},",
    welcomeTagline: "a place for sharing tech.",
    articleCount: "{n} articles in English",
    articleCountOne: "{n} article in English",
    featured: "From today's featured article",
    fullArticle: "Read the full article →",
    recentArticles: "Recent articles",
    noPosts: "There are no articles yet.",

    infoPublished: "Published",
    infoLanguage: "Language",
    infoAlsoIn: "Also available in",
    infoReadingTime: "Reading time",
    minutes: "{n} min",

    search: "Search",
    searchPlaceholder: "Search GoBlog.dev",
    searchResults: "Search results",
    searchPrompt: "Enter a word or phrase to search the articles.",
    searchCount: "{n} results for “{q}”",
    searchCountOne: "{n} result for “{q}”",
    searchNoResults: "There were no results matching “{q}”.",

    notFoundTitle: "Page not found",
    notFoundBody: "GoBlog.dev does not have an article with this exact name.",
    notFoundHint: "Try searching for it, or return to the {link}.",

    footerPublished: "This article was published on {date}.",

    months: ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"],
    dateFormat: "{month} {day}, {year}",
  },
  id: {
    siteTitle: "GoBlog.dev",
    siteTagline: "Berbagi teknologi",
    siteDescription: "Tempat berbagi seputar teknologi, dalam bahasa Indonesia dan Inggris.",
    fromSite: "Dari GoBlog.dev, tempat berbagi seputar teknologi",
    languageName: "Bahasa Indonesia",

    categories: "Kategori",
    category: "Kategori",
    tags: "Tag",
    otherArticles: "Artikel lainnya",
    viewAll: "Lihat semua {n} →",
    categoryTitle: "{name}",
    categoryIntro: "Artikel dalam kategori {name}.",
    tagTitle: "Tag “{tag}”",
    tagIntro: "Artikel dengan tag {tag}.",
    allCategories: "Semua kategori",
    allTags: "Semua tag",
    searchText: "Kata yang dicari",
    applyFilters: "Tampilkan artikel",
    clearFilters: "Hapus filter",
    popularTags: "Jelajahi menurut tag",
    browseByCategory: "Jelajahi menurut kategori",
    filteredCount: "{n} artikel",
    filteredCountOne: "{n} artikel",
    noMatches: "Tidak ada artikel yang cocok dengan filter ini.",
    jumpToContent: "Lompat ke isi",
    appearance: "Tampilan",
    color: "Warna",
    themeAuto: "Otomatis",
    themeLight: "Terang",
    themeDark: "Gelap",
    mainMenu: "Menu utama",
    mainPage: "Halaman Utama",
    about: "Tentang",
    aboutTitle: "Tentang saya",
    infoLocation: "Lokasi",
    infoEmail: "Email",
    infoLinks: "Tautan",
    profileEmpty: "Pemilik blog belum menulis profil.",
    profileFallback: "Profil ini belum diterjemahkan ke dalam bahasa Indonesia.",
    contents: "Daftar isi",
    top: "(Awal)",
    article: "Artikel",
    read: "Baca",
    languagesOne: "{n} bahasa",
    languagesMany: "{n} bahasa",

    welcome: "Selamat datang di {site},",
    welcomeTagline: "tempat berbagi seputar teknologi.",
    articleCount: "{n} artikel dalam bahasa Indonesia",
    articleCountOne: "{n} artikel dalam bahasa Indonesia",
    featured: "Artikel pilihan hari ini",
    fullArticle: "Baca selengkapnya →",
    recentArticles: "Artikel terbaru",
    noPosts: "Belum ada artikel.",

    infoPublished: "Diterbitkan",
    infoLanguage: "Bahasa",
    infoAlsoIn: "Tersedia juga dalam",
    infoReadingTime: "Waktu baca",
    minutes: "{n} menit",

    search: "Cari",
    searchPlaceholder: "Cari di GoBlog.dev",
    searchResults: "Hasil pencarian",
    searchPrompt: "Masukkan kata atau frasa untuk mencari artikel.",
    searchCount: "{n} hasil untuk “{q}”",
    searchCountOne: "{n} hasil untuk “{q}”",
    searchNoResults: "Tidak ada hasil yang cocok dengan “{q}”.",

    notFoundTitle: "Halaman tidak ditemukan",
    notFoundBody: "GoBlog.dev tidak memiliki artikel dengan nama persis seperti ini.",
    notFoundHint: "Coba cari artikelnya, atau kembali ke {link}.",

    footerPublished: "Artikel ini diterbitkan pada {date}.",

    months: ["Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"],
    dateFormat: "{day} {month} {year}",
  },
} satisfies Record<Locale, Record<string, string | string[]>>;

export type Dictionary = (typeof dictionaries)[Locale];

export const getDictionary = (locale: Locale): Dictionary => dictionaries[locale];

// Replaces {name} placeholders in a dictionary string.
export function format(template: string, values: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (match, key: string) => (key in values ? String(values[key]) : match));
}
