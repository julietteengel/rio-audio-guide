/** Lowercases and strips diacritics (é -> e, ã -> a, ç -> c...) so a search
 * query matches regardless of accent -- place names stay in their original
 * language (often Portuguese) while the app's UI, and the person typing,
 * may be in French/English/Spanish. */
export function normalizeForSearch(s: string): string {
  return s
    .trim()
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "");
}
