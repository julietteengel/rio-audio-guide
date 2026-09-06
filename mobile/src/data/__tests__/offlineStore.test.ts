import { hasSufficientStorage, planResumableAudioDownloads } from "../offlineStore";
import type { CachedPlace } from "../offlineStore";

describe("hasSufficientStorage", () => {
  it("is true when available space meets the requirement exactly", () => {
    expect(hasSufficientStorage(1000, 1000)).toBe(true);
  });

  it("is true when available space exceeds the requirement", () => {
    expect(hasSufficientStorage(2000, 1000)).toBe(true);
  });

  it("is false when available space is short", () => {
    expect(hasSufficientStorage(500, 1000)).toBe(false);
  });
});

describe("planResumableAudioDownloads", () => {
  const cachedWithAudio: CachedPlace = {
    id: "a",
    name: "A",
    category: "monument",
    lat: 0,
    lon: 0,
    body: "text",
    audioLocalUri: "file:///a.mp3",
  };
  const cachedWithoutAudio: CachedPlace = { ...cachedWithAudio, id: "b", audioLocalUri: null };

  it("skips places already cached with audio", () => {
    const result = planResumableAudioDownloads(
      [{ id: "a" }, { id: "b" }],
      [cachedWithAudio, cachedWithoutAudio],
    );
    expect(result).toEqual([{ id: "b" }]);
  });

  it("includes a place with no cache row at all", () => {
    const result = planResumableAudioDownloads([{ id: "c" }], [cachedWithAudio]);
    expect(result).toEqual([{ id: "c" }]);
  });

  it("returns everything when nothing is cached yet", () => {
    const manifest = [{ id: "a" }, { id: "b" }];
    expect(planResumableAudioDownloads(manifest, [])).toEqual(manifest);
  });
});
