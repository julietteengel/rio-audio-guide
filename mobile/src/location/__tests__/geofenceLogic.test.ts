import {
  selectNearestRegions,
  shouldReregister,
  isCooldownActive,
  NOTIFICATION_COOLDOWN_MS,
  GEOFENCE_RADIUS_METERS,
  type GeofenceCandidate,
} from "../geofenceLogic";

// Roughly Rio de Janeiro coordinates, spread a few km apart -- exact values
// don't matter, only relative distances do.
const CRISTO: GeofenceCandidate = { id: "cristo", name: "Cristo Redentor", lat: -22.9519, lon: -43.2105 };
const PAO_DE_ACUCAR: GeofenceCandidate = { id: "pao", name: "Pão de Açúcar", lat: -22.9491, lon: -43.1545 };
const SELARON: GeofenceCandidate = { id: "selaron", name: "Escadaria Selarón", lat: -22.9147, lon: -43.1796 };
const CENTER_NEAR_CRISTO = { latitude: -22.952, longitude: -43.211 };

describe("selectNearestRegions", () => {
  it("returns the closest place first", () => {
    const regions = selectNearestRegions([PAO_DE_ACUCAR, CRISTO, SELARON], CENTER_NEAR_CRISTO);
    expect(regions[0].identifier).toBe("cristo");
  });

  it("caps the result at the given limit", () => {
    const regions = selectNearestRegions([CRISTO, PAO_DE_ACUCAR, SELARON], CENTER_NEAR_CRISTO, 2);
    expect(regions).toHaveLength(2);
  });

  it("defaults the limit to MAX_MONITORED_REGIONS", () => {
    const manyPlaces: GeofenceCandidate[] = Array.from({ length: 30 }, (_, i) => ({
      id: `place-${i}`,
      name: `Place ${i}`,
      lat: -22.95 + i * 0.001,
      lon: -43.2 + i * 0.001,
    }));
    expect(selectNearestRegions(manyPlaces, CENTER_NEAR_CRISTO)).toHaveLength(20);
  });

  it("sets notifyOnEnter true and notifyOnExit false on every region", () => {
    const regions = selectNearestRegions([CRISTO], CENTER_NEAR_CRISTO);
    expect(regions[0].notifyOnEnter).toBe(true);
    expect(regions[0].notifyOnExit).toBe(false);
  });

  it("uses the place id as the region identifier and GEOFENCE_RADIUS_METERS as the radius", () => {
    const regions = selectNearestRegions([CRISTO], CENTER_NEAR_CRISTO);
    expect(regions[0].identifier).toBe("cristo");
    expect(regions[0].radius).toBe(GEOFENCE_RADIUS_METERS);
  });

  it("returns an empty array for an empty place list", () => {
    expect(selectNearestRegions([], CENTER_NEAR_CRISTO)).toEqual([]);
  });
});

describe("shouldReregister", () => {
  it("is true when there is no previously-registered center", () => {
    expect(shouldReregister(null, CENTER_NEAR_CRISTO)).toBe(true);
  });

  it("is false when the user hasn't moved far from the last registered center", () => {
    const barelyMoved = { latitude: CENTER_NEAR_CRISTO.latitude + 0.0001, longitude: CENTER_NEAR_CRISTO.longitude };
    expect(shouldReregister(CENTER_NEAR_CRISTO, barelyMoved)).toBe(false);
  });

  it("is true once the user has moved past REREGISTER_THRESHOLD_METERS", () => {
    // Roughly 1.1km north -- well past the 500m threshold.
    const movedFar = { latitude: CENTER_NEAR_CRISTO.latitude + 0.01, longitude: CENTER_NEAR_CRISTO.longitude };
    expect(shouldReregister(CENTER_NEAR_CRISTO, movedFar)).toBe(true);
  });
});

describe("isCooldownActive", () => {
  it("is false when the place has never been notified about", () => {
    expect(isCooldownActive(null, Date.now())).toBe(false);
  });

  it("is true immediately after a notification", () => {
    const now = Date.now();
    expect(isCooldownActive(now, now)).toBe(true);
  });

  it("is true just before the cooldown window elapses", () => {
    const now = Date.now();
    expect(isCooldownActive(now - NOTIFICATION_COOLDOWN_MS + 1000, now)).toBe(true);
  });

  it("is false once the cooldown window has elapsed", () => {
    const now = Date.now();
    expect(isCooldownActive(now - NOTIFICATION_COOLDOWN_MS - 1000, now)).toBe(false);
  });
});
