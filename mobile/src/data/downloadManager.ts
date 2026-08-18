import type { Locale } from "../i18n/dictionary";
import type { Place } from "./types";

export type DownloadFileRef = {
  placeId: string;
  kind: "metadata" | "audio";
  locale?: Locale;
  path: string;
};

/**
 * Pure function: given the places belonging to a city and the single language the
 * user wants offline, compute exactly which files a "download this city" action
 * needs to fetch. Deliberately fetches only `locale`, never all four, per the
 * design spec (offline packs are per-language, not per-language-times-four).
 */
export function planCityDownload(
  places: Place[],
  city: string,
  locale: Locale,
): DownloadFileRef[] {
  const cityPlaces = places.filter((p) => p.city === city);

  const files: DownloadFileRef[] = [];
  for (const place of cityPlaces) {
    files.push({ placeId: place.id, kind: "metadata", path: `${place.id}/metadata.json` });
    files.push({
      placeId: place.id,
      kind: "audio",
      locale,
      path: `${place.id}/audio-${locale}.mp3`,
    });
  }
  return files;
}

export function estimateDownloadSizeBytes(files: DownloadFileRef[]): number {
  const METADATA_BYTES = 4_000;
  const AUDIO_BYTES_PER_FILE = 1_800_000;
  return files.reduce(
    (total, f) => total + (f.kind === "audio" ? AUDIO_BYTES_PER_FILE : METADATA_BYTES),
    0,
  );
}
