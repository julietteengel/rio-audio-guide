import { API_BASE_URL } from "../config";

export type ItineraryStopKind = "place" | "suggestion";

// A discriminated union, not a flat optional `placeId`, so a suggestion stop
// carrying a `placeId` (or a place stop missing one) is a compile error, not
// a representable-but-illegal state -- mirrors the backend domain's own
// NewPlaceStop/NewSuggestionStop split (internal/domain/itinerary.go).
export type ItineraryStop =
  | {
      kind: "place";
      placeId: string;
      label: string;
      timeOnSiteMinutes: number;
      walkToNextMinutes: number;
    }
  | {
      kind: "suggestion";
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

function stopFromWire(s: WireStop): ItineraryStop {
  if (s.kind === "suggestion") {
    return {
      kind: "suggestion",
      label: s.label,
      timeOnSiteMinutes: s.time_on_site_minutes,
      walkToNextMinutes: s.walk_to_next_minutes,
    };
  }
  return {
    kind: "place",
    // The backend always sends a real place_id for a place-kind stop; the
    // fallback only guards against a malformed/unexpected wire payload, the
    // same defensive posture as the kind-coercion two lines above.
    placeId: s.place_id ?? "",
    label: s.label,
    timeOnSiteMinutes: s.time_on_site_minutes,
    walkToNextMinutes: s.walk_to_next_minutes,
  };
}

function fromWire(wire: WireItinerary): Itinerary {
  return {
    id: wire.id,
    title: wire.title,
    totalMinutes: wire.total_minutes,
    placeCount: wire.place_count,
    stops: wire.stops.map(stopFromWire),
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

// Deliberately does NOT go through itinerariesFetch -- every other function
// in this file requires a token (Authorization header always sent);
// featured itineraries are public, so this is a plain fetch with no auth at
// all, proving out the same "works while logged out" contract the backend
// route itself guarantees.
export async function listFeaturedItineraries(): Promise<Itinerary[]> {
  const res = await fetch(`${API_BASE_URL}/featured-itineraries`);
  if (!res.ok) throw new ItinerariesApiError("request failed", res.status);
  // Guards against a 200 with a non-JSON body (an odd proxy/cache response,
  // an HTML error page served with the wrong status) the same way
  // itinerariesFetch does for every other function here -- without it, a
  // malformed body would throw past this function's intended error type.
  let wire: WireItinerary[];
  try {
    wire = (await res.json()) as WireItinerary[];
  } catch {
    throw new ItinerariesApiError("invalid response body", res.status);
  }
  return wire.map(fromWire);
}
