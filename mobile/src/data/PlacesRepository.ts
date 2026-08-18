import type { Place } from "./types";
import { MOCK_PLACES } from "./types";

export interface PlacesRepository {
  listNearby(): Promise<Place[]>;
  getById(id: string): Promise<Place | undefined>;
  search(query: string): Promise<Place[]>;
  downloadedCount(): Promise<number>;
}

/**
 * Renders every screen against the same fixed example (Cristo Redentor) used
 * throughout the approved design prototype. Swap this for a real HTTP-backed
 * implementation once the backend exposes place-list/detail endpoints — every
 * screen depends on the `PlacesRepository` interface, not this class, so that
 * swap touches this one file. See the mobile app spec's "No backend calls yet"
 * decision and the final report's backend gap list.
 */
export class MockPlacesRepository implements PlacesRepository {
  async listNearby(): Promise<Place[]> {
    return MOCK_PLACES;
  }

  async getById(id: string): Promise<Place | undefined> {
    return MOCK_PLACES.find((p) => p.id === id);
  }

  async search(query: string): Promise<Place[]> {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    return MOCK_PLACES.filter((p) => p.name.toLowerCase().includes(q));
  }

  async downloadedCount(): Promise<number> {
    return MOCK_PLACES.length;
  }
}

export const placesRepository: PlacesRepository = new MockPlacesRepository();
