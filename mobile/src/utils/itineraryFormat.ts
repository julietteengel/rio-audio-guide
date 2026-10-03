import type { Dictionary } from "../i18n/dictionary";

// "1h" / "1h30" / "2h05" / "45 min" -- matches how duration is written
// colloquially in fr/en/pt/es alike (unlike most copy in this app, this
// abbreviation doesn't need a per-locale dictionary entry).
export function formatDuration(totalMinutes: number): string {
  if (totalMinutes < 60) return `${totalMinutes} min`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (minutes === 0) return `${hours}h`;
  return `${hours}h${minutes.toString().padStart(2, "0")}`;
}

// "1h30 · 4 lieux" -- the duration + stop-count summary line shown on every
// itinerary card/detail screen (Map's Discover row, FeaturedItineraryDetail,
// ItineraryDetail). Takes `t` explicitly (rather than calling useLocale()
// itself) so it stays a plain function, usable inside list-item render
// callbacks that already have `t` in scope.
export function formatItinerarySummary(
  itinerary: { totalMinutes: number; placeCount: number },
  t: Dictionary,
): string {
  return `${formatDuration(itinerary.totalMinutes)} · ${t.itineraries.stopCount.replace("{count}", String(itinerary.placeCount))}`;
}
