// Share links for an article. They are plain links to each network's share
// page, so they work without JavaScript and load no third-party scripts. The
// networks read the title, summary and picture from the article's Open Graph
// tags, so the address must be one their crawlers can reach.
export type ShareNetwork = "x" | "facebook" | "linkedin" | "whatsapp" | "telegram" | "email";
export type ShareLink = { network: ShareNetwork; name: string; href: string };

export function shareLinks(url: string, title: string): ShareLink[] {
  const u = encodeURIComponent(url);
  const text = encodeURIComponent(title);
  const both = encodeURIComponent(`${title} ${url}`);
  return [
    { network: "x", name: "X", href: `https://x.com/intent/post?url=${u}&text=${text}` },
    { network: "facebook", name: "Facebook", href: `https://www.facebook.com/sharer/sharer.php?u=${u}` },
    { network: "linkedin", name: "LinkedIn", href: `https://www.linkedin.com/sharing/share-offsite/?url=${u}` },
    { network: "whatsapp", name: "WhatsApp", href: `https://wa.me/?text=${both}` },
    { network: "telegram", name: "Telegram", href: `https://t.me/share/url?url=${u}&text=${text}` },
    { network: "email", name: "Email", href: `mailto:?subject=${text}&body=${u}` },
  ];
}
