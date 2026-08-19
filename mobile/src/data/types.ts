// "ready": narration text is in `body`, safe to render.
// "pending": the backend has this place but no *published* script yet for the
//   requested language (its /places/:id route returned 202) — `body` is empty.
// "unavailable": no script exists at all for this place/language (404) —
//   `body` is empty and no narration will appear until content is authored.
export type NarrationStatus = "ready" | "pending" | "unavailable";

// Same 3 states as NarrationStatus, but for GET /places/:id/audio, which is
// a genuinely separate endpoint/lifecycle from the text narration above —
// a script can be published (narration ready) while its audio is still
// generating, or the other way is never true (audio always needs a
// published script first). "ready" carries the real presigned S3 URL.
export type AudioAvailability =
  | { state: "ready"; url: string }
  | { state: "pending" }
  | { state: "unavailable" };

export type Place = {
  id: string;
  name: string;
  category: string;
  // The backend's Place has no neighborhood/quarter field yet (see the
  // backend gap list) — undefined for anything sourced from HttpPlacesRepository.
  neighborhood?: string;
  lat: number;
  lon: number;
  city: string;
  // Static/mock only. The real app never trusts a stored distance — it's a
  // function of (place, the user's live position), computed in Map.tsx.
  distanceMeters?: number;
  // The backend's audio endpoint doesn't return a duration yet — undefined
  // for anything sourced from HttpPlacesRepository.
  audioDurationSeconds?: number;
  body: string;
  groundedSourceCount: number;
  narrationStatus: NarrationStatus;
};

export const MOCK_PLACES: Place[] = [
  {
    id: "cristo-redentor",
    name: "Cristo Redentor",
    category: "Monument",
    neighborhood: "Zona Sul",
    lat: -22.9519,
    lon: -43.2105,
    city: "Rio de Janeiro",
    distanceMeters: 820,
    audioDurationSeconds: 135,
    body: "Inaugurée en 1931, la statue du Christ Rédempteur culmine à 710 mètres au sommet du Corcovado. Elle est aujourd'hui l'un des symboles les plus reconnus du Brésil.",
    groundedSourceCount: 1,
    narrationStatus: "ready",
  },
];
