/** Pictures the writer picks, pastes or drops, read for the engine: reference
 *  images on a character sheet and images attached to an agent message. Both
 *  take PNG, JPEG, WebP or GIF up to 5 MB. */

export const IMAGE_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"] as const;
export const IMAGE_ACCEPT = IMAGE_TYPES.join(",");
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024;

/** A long side past this is scaled down before sending: a phone photo or a
 *  4K screenshot carries nothing a model or an image provider uses beyond it. */
const MAX_SIDE = 2048;
/** Only pictures heavier than this are worth decoding to check their size. */
const SHRINK_FROM_BYTES = 1.5 * 1024 * 1024;

export type ImageFileError = "unsupported" | "tooLarge";

export interface ReadImage {
  mime: string;
  data_base64: string;
  /** A data: URL of the same bytes, for a preview. */
  url: string;
}

export class ImageFileProblem extends Error {
  constructor(public readonly reason: ImageFileError) {
    super(reason);
  }
}

export function isSupportedImage(file: File): boolean {
  return (IMAGE_TYPES as readonly string[]).includes(file.type);
}

function readDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = () => reject(reader.error ?? new Error("file read failed"));
    reader.readAsDataURL(blob);
  });
}

function fromDataUrl(url: string, fallbackMime: string): ReadImage {
  const comma = url.indexOf(",");
  const mime = /^data:([^;,]+)/.exec(url)?.[1] ?? fallbackMime;
  return { mime, data_base64: url.slice(comma + 1), url };
}

/** The picture scaled to MAX_SIDE as JPEG (PNG when it may be transparent),
 *  or null when it is small enough already or cannot be decoded here. */
async function shrink(file: File): Promise<string | null> {
  if (typeof createImageBitmap !== "function" || typeof document === "undefined") return null;
  try {
    const bitmap = await createImageBitmap(file);
    const scale = Math.min(1, MAX_SIDE / Math.max(bitmap.width, bitmap.height));
    if (scale === 1 && file.size <= MAX_IMAGE_BYTES) {
      bitmap.close();
      return null;
    }
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    bitmap.close();
    const type = file.type === "image/png" ? "image/png" : "image/jpeg";
    const url = canvas.toDataURL(type, 0.9);
    return url.startsWith(`data:${type}`) ? url : null;
  } catch {
    return null;
  }
}

/** Reads one picture, scaling a large one down. A GIF is sent as it is, so
 *  an animation is not flattened. Throws ImageFileProblem for a file the
 *  engine would refuse. */
export async function readImageFile(file: File): Promise<ReadImage> {
  if (!isSupportedImage(file)) throw new ImageFileProblem("unsupported");
  if (file.type !== "image/gif" && file.size > SHRINK_FROM_BYTES) {
    const url = await shrink(file);
    if (url) {
      const out = fromDataUrl(url, file.type);
      if ((out.data_base64.length * 3) / 4 <= MAX_IMAGE_BYTES) return out;
    }
  }
  if (file.size > MAX_IMAGE_BYTES) throw new ImageFileProblem("tooLarge");
  return fromDataUrl(await readDataUrl(file), file.type);
}

/** The image files among what was pasted or dropped. */
export function imageFiles(list: FileList | null | undefined): File[] {
  return Array.from(list ?? []).filter((f) => f.type.startsWith("image/"));
}
