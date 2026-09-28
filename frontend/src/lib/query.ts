// Builds "a=1&b=2" from the non-empty values. URLSearchParams would do, but
// it's a browser API that goja (which renders pages on the server) lacks.
export function queryString(params: Record<string, string>): string {
  return Object.entries(params)
    .filter(([, v]) => v !== "")
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join("&");
}
