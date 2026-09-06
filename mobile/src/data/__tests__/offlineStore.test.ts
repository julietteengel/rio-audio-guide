import {
  hasSufficientStorage,
  planResumableAudioDownloads,
  getLastNotifiedAt,
  setLastNotifiedAt,
  saveCachedPlaces,
  clearCachedPlaces,
} from "../offlineStore";
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

  // expo-file-system's web shim reports 0 free bytes for every platform it
  // doesn't support -- an unknown value, not a full disk.
  it("treats an unknown (non-positive) available space as unenforceable", () => {
    expect(hasSufficientStorage(0, 1000)).toBe(true);
    expect(hasSufficientStorage(-1, 1000)).toBe(true);
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

describe("getLastNotifiedAt / setLastNotifiedAt", () => {
  afterEach(async () => {
    await clearCachedPlaces();
  });

  it("is null for a place that was never notified about", async () => {
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "text", audioLocalUri: null },
    ]);
    expect(await getLastNotifiedAt("cristo")).toBeNull();
  });

  it("returns the timestamp set by setLastNotifiedAt", async () => {
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "text", audioLocalUri: null },
    ]);
    const ts = Date.now();
    await setLastNotifiedAt("cristo", ts);
    expect(await getLastNotifiedAt("cristo")).toBe(ts);
  });

  it("is not clobbered by a later saveCachedPlaces upsert of the same place", async () => {
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "text", audioLocalUri: null },
    ]);
    const ts = Date.now();
    await setLastNotifiedAt("cristo", ts);
    // Re-saving (as a fresh download would) must not reset the cooldown clock.
    await saveCachedPlaces([
      { id: "cristo", name: "Cristo Redentor", category: "monument", lat: -22.9519, lon: -43.2105, body: "updated text", audioLocalUri: null },
    ]);
    expect(await getLastNotifiedAt("cristo")).toBe(ts);
  });
});
