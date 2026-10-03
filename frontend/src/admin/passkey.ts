// Passkey (WebAuthn) ceremonies for the admin. The server sends options as
// JSON with binary fields base64url-encoded; the browser API needs them as
// ArrayBuffers, and its answer has to go back the other way.

export const passkeysSupported = () => typeof window !== "undefined" && typeof window.PublicKeyCredential === "function";

function fromB64url(s: string): ArrayBuffer {
  const b64 = s.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((s.length + 3) % 4);
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
}

function toB64url(buf: ArrayBuffer | null): string | undefined {
  if (!buf) return undefined;
  let bin = "";
  for (const b of new Uint8Array(buf)) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

type Descriptor = { id: string; type: string; transports?: string[] };

async function post<T>(url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = (await res.json().catch(() => ({}))) as T & { error?: string };
  if (!res.ok) throw new Error(data.error ?? "Something went wrong. Try again.");
  return data;
}

// Turns the browser's errors into something worth showing.
function explain(e: unknown, action: "sign in" | "add"): Error {
  const name = e instanceof DOMException ? e.name : "";
  if (name === "NotAllowedError" || name === "AbortError") {
    return new Error(action === "sign in" ? "Passkey sign-in was cancelled, or no passkey for this site was found." : "Adding the passkey was cancelled.");
  }
  if (name === "InvalidStateError") return new Error("This device already has a passkey for this admin.");
  if (name === "SecurityError") return new Error("Passkeys need the site to be opened over https (or on localhost).");
  return e instanceof Error ? e : new Error("Something went wrong. Try again.");
}

// Signs in with a passkey the device offers; resolves with where to go next.
export async function signInWithPasskey(): Promise<string> {
  const { publicKey } = await post<{ publicKey: Record<string, unknown> & { challenge: string; allowCredentials?: Descriptor[] } }>(
    "/admin/passkey/login/begin",
  );
  let cred: PublicKeyCredential;
  try {
    cred = (await navigator.credentials.get({
      publicKey: {
        ...publicKey,
        challenge: fromB64url(publicKey.challenge),
        allowCredentials: publicKey.allowCredentials?.map((d) => ({ ...d, id: fromB64url(d.id) })),
      } as PublicKeyCredentialRequestOptions,
    })) as PublicKeyCredential;
  } catch (e) {
    throw explain(e, "sign in");
  }
  const r = cred.response as AuthenticatorAssertionResponse;
  const { redirect } = await post<{ redirect: string }>("/admin/passkey/login/finish", {
    id: cred.id,
    rawId: toB64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: {
      clientDataJSON: toB64url(r.clientDataJSON),
      authenticatorData: toB64url(r.authenticatorData),
      signature: toB64url(r.signature),
      userHandle: toB64url(r.userHandle),
    },
  });
  return redirect;
}

// Creates a passkey on this device and saves it under name.
export async function addPasskey(name: string): Promise<void> {
  const { publicKey } = await post<{
    publicKey: Record<string, unknown> & { challenge: string; user: { id: string; name: string; displayName: string }; excludeCredentials?: Descriptor[] };
  }>("/admin/passkeys/register/begin");
  let cred: PublicKeyCredential;
  try {
    cred = (await navigator.credentials.create({
      publicKey: {
        ...publicKey,
        challenge: fromB64url(publicKey.challenge),
        user: { ...publicKey.user, id: fromB64url(publicKey.user.id) },
        excludeCredentials: publicKey.excludeCredentials?.map((d) => ({ ...d, id: fromB64url(d.id) })),
      } as PublicKeyCredentialCreationOptions,
    })) as PublicKeyCredential;
  } catch (e) {
    throw explain(e, "add");
  }
  const r = cred.response as AuthenticatorAttestationResponse;
  await post("/admin/passkeys/register/finish", {
    name,
    credential: {
      id: cred.id,
      rawId: toB64url(cred.rawId),
      type: cred.type,
      authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
      clientExtensionResults: cred.getClientExtensionResults(),
      response: {
        clientDataJSON: toB64url(r.clientDataJSON),
        attestationObject: toB64url(r.attestationObject),
        transports: typeof r.getTransports === "function" ? r.getTransports() : undefined,
      },
    },
  });
}

// A name suggestion for a new passkey, from the device it's created on.
export function suggestPasskeyName(): string {
  const ua = typeof navigator === "undefined" ? "" : navigator.userAgent;
  if (/iPhone/.test(ua)) return "iPhone";
  if (/iPad/.test(ua)) return "iPad";
  if (/Android/.test(ua)) return "Android phone";
  if (/Macintosh/.test(ua)) return "Mac";
  if (/Windows/.test(ua)) return "Windows PC";
  return "Passkey";
}
