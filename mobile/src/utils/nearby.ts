import type { Place } from "../data/types";
import { haversineMeters, type LatLon } from "./geo";

/** Beyond this, "within earshot" would be false (e.g. someone browsing the app from abroad). */
export const NEARBY_REACH_METERS = 5000;

export function isWithinReach(meters: number): boolean {
  return meters <= NEARBY_REACH_METERS;
}

export type Nearby = { place: Place; distanceMeters: number | null };

function closestTo(places: Place[], from: LatLon): Nearby {
  let closest = places[0];
  let closestDistance = haversineMeters(from, { latitude: closest.lat, longitude: closest.lon });
  for (const p of places.slice(1)) {
    const d = haversineMeters(from, { latitude: p.lat, longitude: p.lon });
    if (d < closestDistance) {
      closest = p;
      closestDistance = d;
    }
  }
  return { place: closest, distanceMeters: closestDistance };
}

/**
 * The place to suggest on the map: the closest one to the user that has
 * listenable audio, falling back to the closest overall when none does (or
 * when the audio-ready set has not loaded yet). With no location, the first
 * place and no distance rather than a fabricated number.
 */
export function pickNearby(
  places: Place[],
  audioReadyIds: Set<string> | null,
  userLocation: LatLon | null,
): Nearby | null {
  if (places.length === 0) return null;
  if (!userLocation) return { place: places[0], distanceMeters: null };
  const withAudio = audioReadyIds ? places.filter((p) => audioReadyIds.has(p.id)) : [];
  return closestTo(withAudio.length > 0 ? withAudio : places, userLocation);
}
