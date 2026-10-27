import {
  planCityDownload,
  estimateDownloadSizeBytes,
  fetchCityManifest,
  formatApproxSize,
  InsufficientStorageError,
  isRetryableDownloadError,
} from "../downloadManager";
import type { Place } from "../types";

const RIO_PLACE: Place = {
  id: "cristo-redentor",
  name: "Cristo Redentor",
  category: "Monument",
  neighborhood: "Zona Sul",
  lat: -22.9519,
  lon: -43.2105,
  city: "Rio de Janeiro",
  distanceMeters: 820,
  audioDurationSeconds: 135,
  body: "test",
  groundedSourceCount: 1,
  narrationStatus: "ready",
};

const OTHER_CITY_PLACE: Place = { ...RIO_PLACE, id: "other", city: "São Paulo" };

describe("planCityDownload", () => {
  it("returns no files for an empty place list", () => {
    expect(planCityDownload([], "Rio de Janeiro", "en")).toEqual([]);
  });

  it("only includes places belonging to the requested city", () => {
    const plan = planCityDownload([RIO_PLACE, OTHER_CITY_PLACE], "Rio de Janeiro", "en");
    expect(plan.every((f) => f.placeId === "cristo-redentor")).toBe(true);
  });

  it("requests exactly one audio file, in the requested locale only", () => {
    const plan = planCityDownload([RIO_PLACE], "Rio de Janeiro", "pt");
    const audioFiles = plan.filter((f) => f.kind === "audio");
    expect(audioFiles).toHaveLength(1);
    expect(audioFiles[0].locale).toBe("pt");
  });

  it("requests exactly one metadata file per place, with no locale", () => {
    const plan = planCityDownload([RIO_PLACE], "Rio de Janeiro", "en");
    const metaFiles = plan.filter((f) => f.kind === "metadata");
    expect(metaFiles).toHaveLength(1);
    expect(metaFiles[0].locale).toBeUndefined();
  });

  it("never requests a language the user did not select", () => {
    const plan = planCityDownload([RIO_PLACE], "Rio de Janeiro", "fr");
    const locales = new Set(plan.filter((f) => f.locale).map((f) => f.locale));
    expect(locales).toEqual(new Set(["fr"]));
  });
});

describe("estimateDownloadSizeBytes", () => {
  it("is zero for an empty plan", () => {
    expect(estimateDownloadSizeBytes([])).toBe(0);
  });

  it("grows with the number of audio files", () => {
    const onePlace = planCityDownload([RIO_PLACE], "Rio de Janeiro", "en");
    const twoPlaces = planCityDownload([RIO_PLACE, { ...RIO_PLACE, id: "b" }], "Rio de Janeiro", "en");
    expect(estimateDownloadSizeBytes(twoPlaces)).toBeGreaterThan(
      estimateDownloadSizeBytes(onePlace),
    );
  });
});

describe("formatApproxSize", () => {
  it("uses Mo for French and MB for the other three languages", () => {
    expect(formatApproxSize(184_000_000, "fr")).toBe("184 Mo");
    expect(formatApproxSize(184_000_000, "en")).toBe("184 MB");
    expect(formatApproxSize(184_000_000, "pt")).toBe("184 MB");
    expect(formatApproxSize(184_000_000, "es")).toBe("184 MB");
  });

  it("keeps one decimal place under 10 MB", () => {
    expect(formatApproxSize(4_200_000, "en")).toBe("4.2 MB");
  });
});

describe("fetchCityManifest", () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("maps a successful manifest response to Place[] with narrationStatus ready", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue({
      status: 200,
      json: async () => ({
        city: "rio",
        language: "fr",
        places: [
          {
            id: "cristo-redentor",
            name: "Cristo Redentor",
            category: "monument",
            lat: -22.9519,
            lon: -43.2105,
            narration: "Inaugurée en 1931...",
            source: "wikidata",
            source_richness: "rich",
            audio_url: "https://example.com/audio.mp3",
          },
        ],
      }),
    }) as unknown as typeof fetch;

    const places = await fetchCityManifest("rio", "fr");

    expect(places).toHaveLength(1);
    expect(places[0]).toMatchObject({
      id: "cristo-redentor",
      name: "Cristo Redentor",
      body: "Inaugurée en 1931...",
      narrationStatus: "ready",
      groundedSourceCount: 1,
    });
  });

  it("returns an empty array when the backend responds with a non-200 status", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue({ status: 404 }) as unknown as typeof fetch;
    expect(await fetchCityManifest("unknown-city", "fr")).toEqual([]);
  });

  it("returns an empty array instead of throwing on a network failure, after retrying", async () => {
    jest.useFakeTimers();
    try {
      globalThis.fetch = jest.fn().mockRejectedValue(new Error("network down")) as unknown as typeof fetch;
      const result = fetchCityManifest("rio", "fr");
      await jest.runAllTimersAsync();
      expect(await result).toEqual([]);
      expect(globalThis.fetch).toHaveBeenCalledTimes(3);
    } finally {
      jest.useRealTimers();
    }
  });

  it("recovers when the first attempt hits a transient 503", async () => {
    jest.useFakeTimers();
    try {
      globalThis.fetch = jest
        .fn()
        .mockResolvedValueOnce({ status: 503 })
        .mockResolvedValue({
          status: 200,
          json: async () => ({ city: "rio", language: "fr", places: [] }),
        }) as unknown as typeof fetch;
      const result = fetchCityManifest("rio", "fr");
      await jest.runAllTimersAsync();
      expect(await result).toEqual([]);
      expect(globalThis.fetch).toHaveBeenCalledTimes(2);
    } finally {
      jest.useRealTimers();
    }
  });
});

describe("isRetryableDownloadError", () => {
  it("does not retry an expired or missing presigned URL", () => {
    expect(isRetryableDownloadError(new Error("Request failed with status 403"))).toBe(false);
    expect(isRetryableDownloadError(new Error("HTTP 404 Not Found"))).toBe(false);
  });

  it("does not mistake a port number for an HTTP status", () => {
    expect(isRetryableDownloadError(new Error("failed to connect to host:443"))).toBe(true);
    expect(isRetryableDownloadError(new Error("downloaded 404 of 1200 bytes, connection reset"))).toBe(true);
  });

  it("retries rate limiting, timeouts, server errors and dropped connections", () => {
    expect(isRetryableDownloadError(new Error("status 429"))).toBe(true);
    expect(isRetryableDownloadError(new Error("status 408"))).toBe(true);
    expect(isRetryableDownloadError(new Error("status 503"))).toBe(true);
    expect(isRetryableDownloadError(new Error("The network connection was lost"))).toBe(true);
    expect(isRetryableDownloadError("something odd")).toBe(true);
  });
});

describe("InsufficientStorageError", () => {
  it("carries the required and available byte counts", () => {
    const err = new InsufficientStorageError(500_000_000, 100_000_000);
    expect(err.requiredBytes).toBe(500_000_000);
    expect(err.availableBytes).toBe(100_000_000);
    expect(err.name).toBe("InsufficientStorageError");
  });
});
