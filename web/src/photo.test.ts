import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cropAndCompressPhoto,
  movePhotoCrop,
  PHOTO_SOURCE_MAX_BYTES,
  photoCropFrameGeometry,
  validatePhotoSource,
} from "./photo";

describe("photo processing", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("rejects unsupported and oversized source files", () => {
    expect(validatePhotoSource(new File(["x"], "photo.webp", { type: "image/webp" })))
      .toContain("JPEG или PNG");
    const oversized = new File([new Uint8Array(PHOTO_SOURCE_MAX_BYTES + 1)], "photo.jpg", {
      type: "image/jpeg",
    });
    expect(validatePhotoSource(oversized)).toContain("20 МБ");
  });

  it("crops to a square and returns a compressed JPEG", async () => {
    const close = vi.fn();
    vi.stubGlobal("createImageBitmap", vi.fn().mockResolvedValue({
      width: 4000,
      height: 3000,
      close,
    }));
    const drawImage = vi.fn();
    const fillRect = vi.fn();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage,
      fillRect,
      fillStyle: "",
    } as unknown as CanvasRenderingContext2D);
    vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((callback) => {
      callback(new Blob(["compressed"], { type: "image/jpeg" }));
    });

    const result = await cropAndCompressPhoto(
      new File(["source"], "weekend.png", { type: "image/png" }),
      { x: 75, y: 25, zoom: 2 },
    );

    expect(result.type).toBe("image/jpeg");
    expect(result.name).toBe("weekend-ryden.jpg");
    expect(drawImage).toHaveBeenCalledOnce();
    expect(HTMLCanvasElement.prototype.getContext).toHaveBeenCalled();
    expect(close).toHaveBeenCalledOnce();
  });

  it("positions a movable square over the full rendered photo", () => {
    expect(photoCropFrameGeometry(
      { x: 75, y: 25, zoom: 2 },
      4000,
      3000,
    )).toEqual({
      left: 46.875,
      top: 12.5,
      width: 37.5,
      height: 50,
    });

    const moved = movePhotoCrop(
      { x: 40, y: 20, zoom: 2 },
      50,
      30,
      400,
      300,
    );
    expect(moved.x).toBeCloseTo(60);
    expect(moved.y).toBeCloseTo(40);
  });
});
