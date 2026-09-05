export type WordMark = { time: number; value: string };

type RawMark = { time?: number; type?: string; value?: string };

/** Parses Polly's NDJSON speech marks, keeping only "word" entries. A
 * corrupted line is skipped rather than failing the whole file — one
 * mistimed word shouldn't cost the narration all highlighting. */
export function parseWordMarks(ndjson: string): WordMark[] {
  const marks: WordMark[] = [];
  for (const line of ndjson.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    let raw: unknown;
    try {
      raw = JSON.parse(trimmed);
    } catch {
      continue;
    }
    if (raw === null || typeof raw !== "object") continue;
    const { type, time, value } = raw as RawMark;
    if (type !== "word" || typeof time !== "number" || typeof value !== "string") {
      continue;
    }
    marks.push({ time, value });
  }
  return marks;
}

/** Binary search for the last index with marks[i].time <= currentTimeMs.
 * -1 if currentTimeMs precedes the first mark, or marks is empty. */
export function findActiveWordIndex(marks: WordMark[], currentTimeMs: number): number {
  if (marks.length === 0 || currentTimeMs < marks[0].time) return -1;
  let lo = 0;
  let hi = marks.length - 1;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (marks[mid].time <= currentTimeMs) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return lo;
}

/** Never throws — any network or parse failure becomes null, which drives
 * the static-text fallback in SyncedNarration without a separate error state. */
export async function fetchWordMarks(url: string): Promise<WordMark[] | null> {
  try {
    const res = await fetch(url);
    if (!res.ok) return null;
    const text = await res.text();
    return parseWordMarks(text);
  } catch {
    return null;
  }
}

export type NarrationLine = { time: number; text: string };

// Groups consecutive word marks into short lyric-style lines -- greedily
// appends words until the joined text would exceed targetChars, then
// starts a new line. Never flushes a line holding fewer than 2 words
// (forces the next word in instead) so a line never ends up as a single
// orphan word when there's another word available to join it -- this is
// what actually fixes the "short word flashes past unseen" complaint from
// per-word highlighting: a line lasts several seconds, comfortably longer
// than one playback-status poll interval.
export function groupMarksIntoLines(marks: WordMark[], targetChars = 40): NarrationLine[] {
  if (marks.length === 0) return [];

  const lines: NarrationLine[] = [];
  let current: WordMark[] = [marks[0]];

  const flush = () => {
    lines.push({
      time: current[0].time,
      text: current.map((m) => m.value).join(" "),
    });
  };

  for (let i = 1; i < marks.length; i++) {
    const candidate = [...current, marks[i]].map((m) => m.value).join(" ");
    if (candidate.length > targetChars && current.length >= 2) {
      flush();
      current = [marks[i]];
    } else {
      current.push(marks[i]);
    }
  }
  flush();

  return lines;
}

// Same binary-search shape as findActiveWordIndex above, applied to
// NarrationLine[] instead of WordMark[] -- kept as a separate function
// (not a generic) so each call site's intent stays obvious from its name.
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
