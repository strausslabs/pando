export function tryPrettyJSON(text: string): string | null {
  const trimmed = text.trim();
  if (trimmed.length < 2) return null;
  const first = trimmed[0];
  const last = trimmed[trimmed.length - 1];
  const looksObject = first === "{" && last === "}";
  const looksArray = first === "[" && last === "]";
  if (!looksObject && !looksArray) return null;
  try {
    const parsed = JSON.parse(trimmed);
    if (typeof parsed !== "object" || parsed === null) return null;
    return JSON.stringify(parsed, null, 2);
  } catch {
    return null;
  }
}

export function matchesQuery(text: string, query: string): boolean {
  if (!query) return true;
  return text.toLowerCase().includes(query.toLowerCase());
}
