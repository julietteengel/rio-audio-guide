import { HttpPlacesRepository } from "../PlacesRepository";
import * as offlineStore from "../offlineStore";
import * as network from "../../utils/network";

jest.mock("../offlineStore");
jest.mock("../../utils/network");
jest.mock("../downloadManager", () => ({
  getOfflineDownloadSummary: jest.fn().mockResolvedValue(null),
}));

const mockedIsOnline = network.isOnline as jest.MockedFunction<typeof network.isOnline>;
const mockedGetCachedPlace = offlineStore.getCachedPlace as jest.MockedFunction<
  typeof offlineStore.getCachedPlace
>;

const CACHED_CRISTO = {
  id: "cristo-redentor",
  name: "Cristo Redentor",
  category: "monument",
  lat: -22.9519,
  lon: -43.2105,
  body: "Inaugurée en 1931...",
  audioLocalUri: "file:///offline-audio/cristo-redentor.mp3",
};

describe("HttpPlacesRepository.getAudioUrl", () => {
  const repo = new HttpPlacesRepository(() => "fr");
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    jest.clearAllMocks();
  });

  it("streams the network URL when online", async () => {
    mockedIsOnline.mockResolvedValue(true);
    globalThis.fetch = jest.fn().mockResolvedValue({
      status: 200,
      json: async () => ({
        url: "https://cdn.example.com/audio.mp3",
        timestamps_url: "https://cdn.example.com/marks.json",
      }),
    }) as unknown as typeof fetch;

    expect(await repo.getAudioUrl("cristo-redentor", "fr")).toEqual({
      state: "ready",
      url: "https://cdn.example.com/audio.mp3",
      timestampsUrl: "https://cdn.example.com/marks.json",
    });
  });

  it("serves the cached local file when offline and the place was downloaded", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(CACHED_CRISTO);

    expect(await repo.getAudioUrl("cristo-redentor", "fr")).toEqual({
      state: "ready",
      url: "file:///offline-audio/cristo-redentor.mp3",
    });
  });

  it("reports unavailable when offline and the place was never downloaded", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(null);

    expect(await repo.getAudioUrl("never-downloaded", "fr")).toEqual({ state: "unavailable" });
  });

  it("falls back to the cache when the network request itself fails despite reporting online", async () => {
    mockedIsOnline.mockResolvedValue(true);
    globalThis.fetch = jest.fn().mockRejectedValue(new Error("flaky connection")) as unknown as typeof fetch;
    mockedGetCachedPlace.mockResolvedValue(CACHED_CRISTO);

    expect(await repo.getAudioUrl("cristo-redentor", "fr")).toEqual({
      state: "ready",
      url: "file:///offline-audio/cristo-redentor.mp3",
    });
  });
});

describe("HttpPlacesRepository.getById", () => {
  const repo = new HttpPlacesRepository(() => "fr");
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    jest.clearAllMocks();
  });

  it("returns the network result unchanged when online", async () => {
    mockedIsOnline.mockResolvedValue(true);
    globalThis.fetch = jest.fn().mockResolvedValue({
      status: 200,
      json: async () => ({
        id: "cristo-redentor",
        name: "Cristo Redentor",
        category: "monument",
        lat: -22.9519,
        lon: -43.2105,
        language: "fr",
        narration: "Inaugurée en 1931...",
        source: "wikidata",
        source_richness: "rich",
      }),
    }) as unknown as typeof fetch;

    const result = await repo.getById("cristo-redentor");
    expect(result?.narrationStatus).toBe("ready");
    expect(result?.body).toBe("Inaugurée en 1931...");
  });

  it("falls back to the cached place when offline", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(CACHED_CRISTO);

    const result = await repo.getById("cristo-redentor");
    expect(result).toEqual({
      id: "cristo-redentor",
      name: "Cristo Redentor",
      category: "monument",
      lat: -22.9519,
      lon: -43.2105,
      city: "Rio de Janeiro",
      body: "Inaugurée en 1931...",
      groundedSourceCount: 1,
      narrationStatus: "ready",
    });
  });

  it("returns undefined when offline and the place was never downloaded", async () => {
    mockedIsOnline.mockResolvedValue(false);
    mockedGetCachedPlace.mockResolvedValue(null);

    expect(await repo.getById("never-downloaded")).toBeUndefined();
  });
});
