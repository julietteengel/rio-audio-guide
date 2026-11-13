import { pickNearby, isWithinReach, NEARBY_REACH_METERS } from "../nearby";
import type { Place } from "../../data/types";

const place = (id: string, lat: number, lon: number) => ({ id, lat, lon }) as Place;

// ~1 km apart along a meridian (1 degree of latitude is ~111 km).
const here = { latitude: -22.9, longitude: -43.2 };
const near = place("near", -22.9, -43.2);
const mid = place("mid", -22.91, -43.2);
const far = place("far", -22.99, -43.2);

describe("pickNearby", () => {
  it("returns null when there are no places", () => {
    expect(pickNearby([], null, here)).toBeNull();
  });

  it("falls back to the first place with no distance when there is no location", () => {
    expect(pickNearby([mid, near], null, null)).toEqual({ place: mid, distanceMeters: null });
  });

  it("picks the closest place when audio availability is unknown", () => {
    const result = pickNearby([far, mid, near], null, here);
    expect(result?.place.id).toBe("near");
    expect(result?.distanceMeters).toBeCloseTo(0, 0);
  });

  it("prefers the closest place that has audio over a closer one without", () => {
    const result = pickNearby([near, mid, far], new Set(["mid", "far"]), here);
    expect(result?.place.id).toBe("mid");
  });

  it("falls back to the closest place overall when none has audio", () => {
    const result = pickNearby([far, near], new Set<string>(), here);
    expect(result?.place.id).toBe("near");
  });

  it("does not look at audio when there is no location", () => {
    expect(pickNearby([near, mid], new Set(["mid"]), null)?.place.id).toBe("near");
  });
});

describe("isWithinReach", () => {
  it("is true up to the reach threshold and false beyond it", () => {
    expect(isWithinReach(0)).toBe(true);
    expect(isWithinReach(NEARBY_REACH_METERS)).toBe(true);
    expect(isWithinReach(NEARBY_REACH_METERS + 1)).toBe(false);
  });
});
