export const PHOTO_UPLOAD_MAX_BYTES = 3 * 1024 * 1024;
export const PHOTO_SOURCE_MAX_BYTES = 20 * 1024 * 1024;
export const PHOTO_OUTPUT_WIDTH = 1200;
export const AVATAR_UPLOAD_MAX_BYTES = 512 * 1024;
export const AVATAR_OUTPUT_WIDTH = 512;
export const PHOTO_ASPECT_RATIO = 1;

export interface PhotoCrop {
  x: number;
  y: number;
  zoom: number;
}

export interface PhotoOutputOptions {
  outputWidth?: number;
  maxBytes?: number;
  outputName?: string;
}

export interface PhotoCropFrameGeometry {
  left: number;
  top: number;
  width: number;
  height: number;
}

export function photoCropFrameGeometry(
  crop: PhotoCrop,
  imageWidth: number,
  imageHeight: number,
): PhotoCropFrameGeometry {
  if (imageWidth <= 0 || imageHeight <= 0) {
    return { left: 0, top: 0, width: 100, height: 100 };
  }
  const zoom = clamp(crop.zoom, 1, 3);
  const aspect = imageWidth / imageHeight;
  const width = (aspect >= 1 ? 100 / aspect : 100) / zoom;
  const height = (aspect >= 1 ? 100 : aspect * 100) / zoom;
  return {
    left: (100 - width) * clamp(crop.x, 0, 100) / 100,
    top: (100 - height) * clamp(crop.y, 0, 100) / 100,
    width,
    height,
  };
}

export function movePhotoCrop(
  crop: PhotoCrop,
  deltaX: number,
  deltaY: number,
  renderedWidth: number,
  renderedHeight: number,
): PhotoCrop {
  const frame = photoCropFrameGeometry(crop, renderedWidth, renderedHeight);
  const horizontalTravel = renderedWidth * (100 - frame.width) / 100;
  const verticalTravel = renderedHeight * (100 - frame.height) / 100;
  return {
    ...crop,
    x: horizontalTravel > 0
      ? clamp(crop.x + deltaX / horizontalTravel * 100, 0, 100)
      : 50,
    y: verticalTravel > 0
      ? clamp(crop.y + deltaY / verticalTravel * 100, 0, 100)
      : 50,
  };
}

interface DecodedImage {
  source: CanvasImageSource;
  width: number;
  height: number;
  close: () => void;
}

export function validatePhotoSource(file: File): string {
  if (file.type !== "image/jpeg" && file.type !== "image/png") {
    return "Выберите фото в формате JPEG или PNG.";
  }
  if (file.size < 1 || file.size > PHOTO_SOURCE_MAX_BYTES) {
    return "Исходное фото должно быть не больше 20 МБ.";
  }
  return "";
}

export async function cropAndCompressPhoto(
  file: File,
  crop: PhotoCrop,
  options: PhotoOutputOptions = {},
): Promise<File> {
  const decoded = await decodePhoto(file);
  try {
    if (decoded.width * decoded.height > 80_000_000) {
      throw new Error("Фото слишком большое. Выберите изображение до 80 мегапикселей.");
    }
    const normalized = {
      x: clamp(crop.x, 0, 100),
      y: clamp(crop.y, 0, 100),
      zoom: clamp(crop.zoom, 1, 3),
    };
    const imageAspect = decoded.width / decoded.height;
    const baseWidth = imageAspect > PHOTO_ASPECT_RATIO
      ? decoded.height * PHOTO_ASPECT_RATIO
      : decoded.width;
    const baseHeight = imageAspect > PHOTO_ASPECT_RATIO
      ? decoded.height
      : decoded.width / PHOTO_ASPECT_RATIO;
    const cropWidth = baseWidth / normalized.zoom;
    const cropHeight = baseHeight / normalized.zoom;
    const sourceX = (decoded.width - cropWidth) * normalized.x / 100;
    const sourceY = (decoded.height - cropHeight) * normalized.y / 100;
    const targetWidth = options.outputWidth ?? PHOTO_OUTPUT_WIDTH;
    const maxBytes = options.maxBytes ?? PHOTO_UPLOAD_MAX_BYTES;
    const outputWidth = Math.max(1, Math.min(targetWidth, Math.round(cropWidth)));
    const outputHeight = Math.max(1, Math.round(outputWidth / PHOTO_ASPECT_RATIO));
    const canvas = document.createElement("canvas");
    canvas.width = outputWidth;
    canvas.height = outputHeight;
    const context = canvas.getContext("2d");
    if (!context) {
      throw new Error("Браузер не смог подготовить фото.");
    }
    context.fillStyle = "#f3eee4";
    context.fillRect(0, 0, outputWidth, outputHeight);
    context.drawImage(
      decoded.source,
      sourceX,
      sourceY,
      cropWidth,
      cropHeight,
      0,
      0,
      outputWidth,
      outputHeight,
    );

    let compressed: Blob | null = null;
    for (const quality of [0.84, 0.76, 0.68, 0.58]) {
      compressed = await canvasToBlob(canvas, quality);
      if (compressed.size <= maxBytes) break;
    }
    if (!compressed || compressed.size > maxBytes) {
      throw new Error(`Не удалось сжать фото до ${Math.ceil(maxBytes / 1024)} КБ. Выберите другой кадр.`);
    }
    const baseName = options.outputName ?? (file.name.replace(/\.[^.]+$/u, "") || "photo");
    return new File([compressed], `${baseName}-ryden.jpg`, {
      type: "image/jpeg",
      lastModified: Date.now(),
    });
  } finally {
    decoded.close();
  }
}

async function decodePhoto(file: File): Promise<DecodedImage> {
  if ("createImageBitmap" in window) {
    const bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
    return {
      source: bitmap,
      width: bitmap.width,
      height: bitmap.height,
      close: () => bitmap.close(),
    };
  }
  const objectURL = URL.createObjectURL(file);
  const image = new Image();
  try {
    await new Promise<void>((resolve, reject) => {
      image.onload = () => resolve();
      image.onerror = () => reject(new Error("Не удалось прочитать фото."));
      image.src = objectURL;
    });
    return {
      source: image,
      width: image.naturalWidth,
      height: image.naturalHeight,
      close: () => URL.revokeObjectURL(objectURL),
    };
  } catch (error) {
    URL.revokeObjectURL(objectURL);
    throw error;
  }
}

function canvasToBlob(canvas: HTMLCanvasElement, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob);
      else reject(new Error("Браузер не смог сжать фото."));
    }, "image/jpeg", quality);
  });
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}
