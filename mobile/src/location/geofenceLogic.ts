export type GeofenceCandidate = {
  id: string;
  name: string;
  lat: number;
  lon: number;
};

export type GeofenceRegion = {
  identifier: string;
  latitude: number;
  longitude: number;
  radius: number;
  notifyOnEnter: true;
  notifyOnExit: false;
};

// iOS's hard cap on simultaneously-monitored regions (Android allows up to
// 100, but sharing one number keeps geofenceTask.ts to a single code path
// across platforms).
export const MAX_MONITORED_REGIONS = 20;

// A radius generous enough to trigger reliably on foot in a dense urban
// area (GPS drift, tall buildings) without being so wide it fires blocks
// away from the actual site.
export const GEOFENCE_RADIUS_METERS = 150;

// Re-registering the monitored region set is a real native-state churn
// operation -- only worth doing once the user has moved far enough that the
// currently-monitored "nearest 20" might no longer be accurate.
export const REREGISTER_THRESHOLD_METERS = 500;

// Per-place notification cooldown, per the design spec: don't re-notify for
// the same place while lingering nearby.
export const NOTIFICATION_COOLDOWN_MS = 6 * 60 * 60 * 1000;

const EARTH_RADIUS_METERS = 6371000;

function toRadians(degrees: number): number {
  return (degrees * Math.PI) / 180;
}

function haversineMeters(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
): number {
  const dLat = toRadians(lat2 - lat1);
  const dLon = toRadians(lon2 - lon1);
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRadians(lat1)) * Math.cos(toRadians(lat2)) * Math.sin(dLon / 2) ** 2;
  return EARTH_RADIUS_METERS * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

/**
 * The nearest `limit` places to `center`, as geofence regions ready to pass
 * straight to `Location.startGeofencingAsync`. Always entry-only
 * (`notifyOnExit: false`) -- exits are never reported, to limit notification
 * noise, per the design spec.
 */
export function selectNearestRegions(
  places: GeofenceCandidate[],
  center: { latitude: number; longitude: number },
  limit: number = MAX_MONITORED_REGIONS,
): GeofenceRegion[] {
  return [...places]
    .map((place) => ({
      place,
      distance: haversineMeters(center.latitude, center.longitude, place.lat, place.lon),
    }))
    .sort((a, b) => a.distance - b.distance)
    .slice(0, limit)
    .map(({ place }) => ({
      identifier: place.id,
      latitude: place.lat,
      longitude: place.lon,
      radius: GEOFENCE_RADIUS_METERS,
      notifyOnEnter: true as const,
      notifyOnExit: false as const,
    }));
}

/**
 * Whether the monitored region set is stale enough to recompute and
 * re-register -- true with no prior registration (first run), or once the
 * user has moved past REREGISTER_THRESHOLD_METERS from where the current
 * set was last centered.
 */
export function shouldReregister(
  lastRegisteredCenter: { latitude: number; longitude: number } | null,
  currentPosition: { latitude: number; longitude: number },
): boolean {
  if (!lastRegisteredCenter) return true;
  return (
    haversineMeters(
      lastRegisteredCenter.latitude,
      lastRegisteredCenter.longitude,
      currentPosition.latitude,
      currentPosition.longitude,
    ) >= REREGISTER_THRESHOLD_METERS
  );
}

/**
 * Whether a place's notification cooldown is still active -- never
 * notified (`null`) is never on cooldown.
 */
export function isCooldownActive(lastNotifiedAtMs: number | null, nowMs: number): boolean {
  if (lastNotifiedAtMs === null) return false;
  return nowMs - lastNotifiedAtMs < NOTIFICATION_COOLDOWN_MS;
}
