// A full-size image viewer for the admin: the cover, or the images in an
// article's text. It's a modal <dialog>, so focus stays inside and Esc
// closes it; ← and → step through several images, and a click outside the
// picture closes it too. Shows each image's size in pixels, handy for
// checking a cover against the recommended 1200×630.
import { useEffect, useRef, useState } from "react";

export type ViewerImage = { src: string; alt?: string };

export function ImageViewer({ images, start = 0, onClose }: { images: ViewerImage[]; start?: number; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [index, setIndex] = useState(start);
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);
  const [broken, setBroken] = useState(false);
  const image = images[index];
  const many = images.length > 1;

  useEffect(() => {
    const el = dialog.current;
    if (el && !el.open) el.showModal();
    return () => el?.close();
  }, []);

  const go = (step: number) => {
    setIndex((i) => (i + step + images.length) % images.length);
    setSize(null);
    setBroken(false);
  };

  if (!image) return null;
  return (
    <dialog
      ref={dialog}
      className="image-viewer"
      aria-label="Image preview"
      onClose={onClose}
      onCancel={(e) => {
        e.preventDefault(); // close through onClose, so the parent unmounts it
        onClose();
      }}
      onKeyDown={(e) => {
        if (many && e.key === "ArrowRight") go(1);
        if (many && e.key === "ArrowLeft") go(-1);
      }}
      // A click on the backdrop lands on the dialog itself.
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div className="image-viewer-stage" onClick={(e) => e.target === e.currentTarget && onClose()}>
        {broken ? (
          <p className="image-viewer-broken">This image doesn&apos;t load.</p>
        ) : (
          <img
            key={image.src}
            src={image.src}
            alt={image.alt ?? ""}
            onLoad={(e) => setSize({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
            onError={() => setBroken(true)}
          />
        )}
      </div>
      <div className="image-viewer-bar">
        <span className="image-viewer-info">
          {many && <span>{`${index + 1} / ${images.length}`}</span>}
          {size && <span>{`${size.w} × ${size.h} px`}</span>}
          {image.alt && <span className="image-viewer-caption">{image.alt}</span>}
        </span>
        <span className="image-viewer-actions">
          {many && (
            <>
              <button type="button" className="btn btn-quiet btn-sm" onClick={() => go(-1)} aria-label="Previous image">
                ←
              </button>
              <button type="button" className="btn btn-quiet btn-sm" onClick={() => go(1)} aria-label="Next image">
                →
              </button>
            </>
          )}
          <a className="btn btn-quiet btn-sm" href={image.src} target="_blank" rel="noreferrer">
            Open original ↗
          </a>
          <button type="button" className="btn btn-quiet btn-sm" onClick={onClose} aria-label="Close preview" autoFocus>
            ✕
          </button>
        </span>
      </div>
    </dialog>
  );
}
