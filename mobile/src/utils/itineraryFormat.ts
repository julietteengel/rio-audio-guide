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
