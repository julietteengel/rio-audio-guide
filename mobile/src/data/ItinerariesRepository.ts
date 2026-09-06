import { API_BASE_URL } from "../config";

export type ItineraryStopKind = "place" | "suggestion";

export type ItineraryStop = {
  kind: ItineraryStopKind;
  placeId?: string;
  label: string;
  timeOnSiteMinutes: number;
  walkToNextMinutes: number;
};

export type Itinerary = {
  id: string;
  title: string;
  totalMinutes: number;
  placeCount: number;
  stops: ItineraryStop[];
};

export class ItinerariesApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

type WireStop = {
  kind: string;
  place_id?: string;
  label: string;
  time_on_site_minutes: number;
  walk_to_next_minutes: number;
};

type WireItinerary = {
  id: string;
  title: string;
  total_minutes: number;
  place_count: number;
  stops: WireStop[];
};

function fromWire(wire: WireItinerary): Itinerary {
  return {
    id: wire.id,
    title: wire.title,
    totalMinutes: wire.total_minutes,
    placeCount: wire.place_count,
    stops: wire.stops.map((s) => ({
      kind: s.kind === "suggestion" ? "suggestion" : "place",
      placeId: s.place_id,
      label: s.label,
      timeOnSiteMinutes: s.time_on_site_minutes,
      walkToNextMinutes: s.walk_to_next_minutes,
    })),
  };
}

async function itinerariesFetch<T>(
  path: string,
  options: { method: string; token: string; body?: unknown },
): Promise<T> {
  const headers: Record<string, string> = { Authorization: `Bearer ${options.token}` };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(`${API_BASE_URL}${path}`, {
    method: options.method,
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }

  if (!res.ok) {
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : "request failed";
    throw new ItinerariesApiError(message, res.status);
  }

  return body as T;
}

export async function createItinerary(token: string, request: string): Promise<Itinerary> {
  const wire = await itinerariesFetch<WireItinerary>("/itineraries", {
    method: "POST",
    token,
    body: { request },
  });
  return fromWire(wire);
}

export async function listItineraries(token: string): Promise<Itinerary[]> {
  const wire = await itinerariesFetch<WireItinerary[]>("/itineraries", { method: "GET", token });
  return wire.map(fromWire);
}

export async function getItinerary(token: string, id: string): Promise<Itinerary> {
  const wire = await itinerariesFetch<WireItinerary>(`/itineraries/${encodeURIComponent(id)}`, {
    method: "GET",
    token,
  });
  return fromWire(wire);
}
