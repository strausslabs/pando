// Detect whether a log line is a JSON object/array and, if so, return it
// pretty-printed. Many services emit structured JSON logs; rendering them
// formatted makes them readable instead of a wall of escaped text.
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

// matchesQuery does a case-insensitive substring test, the basis for in-log
// search. Empty query matches everything.
export function matchesQuery(text: string, query: string): boolean {
  if (!query) return true;
  return text.toLowerCase().includes(query.toLowerCase());
}
