export type NarrationLine = { time: number; text: string };

type WhisperSegment = { start?: number; text?: string };
type WhisperVerboseJSON = { segments?: WhisperSegment[] };

/** Parses an OpenAI Whisper `verbose_json` transcription (one entry per
 * spoken sentence/segment, with its own start time) into narration lines.
 * Used instead of Polly's per-word speech marks because neither Google TTS
 * nor OpenAI TTS -- the two providers actually in use -- return timing
 * themselves; Whisper re-transcribes the already-generated audio file to
 * recover it, for either provider's voice, after the fact. A malformed or
 * unexpected body yields [] rather than throwing. */
export function parseNarrationSegments(json: string): NarrationLine[] {
  let raw: unknown;
  try {
    raw = JSON.parse(json);
  } catch {
    return [];
  }
  if (raw === null || typeof raw !== "object") return [];
  const segments = (raw as WhisperVerboseJSON).segments;
  if (!Array.isArray(segments)) return [];

  const lines: NarrationLine[] = [];
  for (const segment of segments) {
    if (typeof segment.start !== "number" || typeof segment.text !== "string") continue;
    const text = segment.text.trim();
    if (!text) continue;
    lines.push({ time: Math.round(segment.start * 1000), text });
  }
  return lines;
}

/** Never throws -- any network or parse failure (including a file with no
 * usable segments) becomes null, which drives SyncedNarration's static-text
 * fallback without a separate error state. */
export async function fetchNarrationLines(url: string): Promise<NarrationLine[] | null> {
  try {
    const res = await fetch(url);
    if (!res.ok) return null;
    const text = await res.text();
    const lines = parseNarrationSegments(text);
    return lines.length > 0 ? lines : null;
  } catch {
    return null;
  }
}

// Binary search: the active line is the last one whose timestamp has
// already passed, so -1 before the first line (nothing active yet) and the
// final line once playback is past every timestamp.
export function findActiveLineIndex(lines: NarrationLine[], currentTimeMs: number): number {
  if (lines.length === 0 || currentTimeMs < lines[0].time) return -1;
  let lo = 0;
  let hi = lines.length - 1;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (lines[mid].time <= currentTimeMs) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return lo;
}
