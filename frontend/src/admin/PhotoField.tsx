import { useRef, useState } from "react";

import { uploadImage } from "./upload";

// Profile photo: upload a file (stored with the blog's other images) or paste
// a direct image address. Shows a live preview and notices broken links.
export function PhotoField({ defaultValue, error }: { defaultValue: string; error?: string }) {
  const [url, setUrl] = useState(defaultValue);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState("");
  const [broken, setBroken] = useState(false);
  const [edited, setEdited] = useState(false); // the server's error is about the old value
  const fileInput = useRef<HTMLInputElement>(null);
  const message = uploadError || (edited ? "" : error);

  function change(next: string) {
    setUrl(next);
    setBroken(false);
    setEdited(true);
  }

  async function onFile(file: File | undefined) {
    if (!file) return;
    setUploading(true);
    setUploadError("");
    try {
      change(await uploadImage(file));
    } catch (e) {
      setUploadError(e instanceof Error ? e.message : "Upload failed.");
    } finally {
      setUploading(false);
    }
  }

  const previewable = /^(https:\/\/|http:\/\/|\/media\/)/.test(url);

  return (
    <div className={message ? "field has-error" : "field"}>
      <span className="field-label">Photo</span>
      <div className="photo-field">
        <div className="photo-preview" aria-hidden="true">
          {previewable && !broken ? <img src={url} alt="" onError={() => setBroken(true)} onLoad={() => setBroken(false)} /> : <span>No photo</span>}
        </div>
        <div className="photo-controls">
          <div className="photo-buttons">
            <button type="button" className="btn btn-primary" onClick={() => fileInput.current?.click()} disabled={uploading}>
              {uploading ? "Uploading…" : "Upload photo"}
            </button>
            {url && (
              <button type="button" className="btn btn-quiet" onClick={() => change("")}>
                Remove
              </button>
            )}
          </div>
          <input
            type="text"
            name="photoUrl"
            value={url}
            onChange={(e) => change(e.target.value.trim())}
            placeholder="…or paste a direct image address (https://…)"
            aria-label="Photo address"
            spellCheck={false}
          />
          <input ref={fileInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp" hidden onChange={(e) => void onFile(e.target.files?.[0])} />
        </div>
      </div>
      {message ? (
        <span className="field-error">{message}</span>
      ) : broken ? (
        <span className="field-error">This address doesn&apos;t load as an image. Links to share pages (Google Drive, Photos, Dropbox) don&apos;t work; use Upload photo.</span>
      ) : (
        <span className="field-hint">A square photo works best. PNG, JPEG, GIF or WebP, up to 5 MB.</span>
      )}
    </div>
  );
}
