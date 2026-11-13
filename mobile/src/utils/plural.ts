/**
 * Fills {count} and picks between the two forms of a "singular|plural" word,
 * e.g. plural("{count} lugar|lugares", 1) -> "1 lugar". Only exactly 1 is
 * singular, which holds for all four locales.
 */
export function plural(template: string, count: number): string {
  return template
    .replace(/([^\s|]+)\|([^\s|]+)/g, (_, one: string, other: string) => (count === 1 ? one : other))
    .replace("{count}", String(count));
}
