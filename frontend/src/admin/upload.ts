// Uploads an image to the blog's media store and returns its URL
// ("/media/<id>.<ext>"). Throws an Error with a readable message on failure.
export async function uploadImage(file: File): Promise<string> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch("/admin/media", { method: "POST", body, credentials: "same-origin", headers: { Accept: "application/json" } });
  if (res.redirected || !res.headers.get("content-type")?.includes("json")) {
    throw new Error("Your session has expired. Sign in again in another tab, then retry.");
  }
  const data = (await res.json()) as { url?: string; error?: string };
  if (!res.ok || !data.url) throw new Error(data.error ?? "Upload failed.");
  return data.url;
}
