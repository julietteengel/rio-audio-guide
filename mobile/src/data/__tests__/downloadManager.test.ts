import { planCityDownload, estimateDownloadSizeBytes } from "../downloadManager";
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
