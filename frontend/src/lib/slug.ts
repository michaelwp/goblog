// slugify turns a title into a URL slug: "Café & Crème, 2024!" -> "cafe-creme-2024".
// It matches the server's slugify and its slug pattern (lowercase letters,
// numbers and single hyphens, up to 100 characters).
export function slugify(title: string): string {
  const slug = title
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "") // drop accents
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  if (slug.length <= 100) return slug;
  // Cut at a word boundary rather than mid-word.
  const cut = slug.slice(0, 100);
  return cut.includes("-") ? cut.slice(0, cut.lastIndexOf("-")) : cut;
}
