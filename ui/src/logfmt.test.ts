import { test, expect, describe } from "bun:test";
import { tryPrettyJSON, matchesQuery } from "./logfmt";

describe("tryPrettyJSON", () => {
  test("pretty-prints a JSON object", () => {
    expect(tryPrettyJSON('{"a":1}')).toBe('{\n  "a": 1\n}');
  });

  test("pretty-prints a JSON array", () => {
    expect(tryPrettyJSON("[1,2]")).toBe("[\n  1,\n  2\n]");
  });

  test("tolerates surrounding whitespace", () => {
    expect(tryPrettyJSON('  {"a":1}  ')).toBe('{\n  "a": 1\n}');
  });

  test("returns null for plain text", () => {
    expect(tryPrettyJSON("just a log line")).toBeNull();
  });

  test("returns null for malformed JSON that merely looks like an object", () => {
    expect(tryPrettyJSON("{not json}")).toBeNull();
  });

  test("returns null for a bare scalar (not object/array)", () => {
    expect(tryPrettyJSON("42")).toBeNull();
    expect(tryPrettyJSON('"hi"')).toBeNull();
  });

  test("returns null for too-short input", () => {
    expect(tryPrettyJSON("{")).toBeNull();
    expect(tryPrettyJSON("")).toBeNull();
  });

  test("does not treat object-open + array-close as JSON", () => {
    expect(tryPrettyJSON("{]")).toBeNull();
  });
});

describe("matchesQuery", () => {
  test("empty query matches everything", () => {
    expect(matchesQuery("anything", "")).toBe(true);
  });

  test("case-insensitive substring", () => {
    expect(matchesQuery("ERROR: boom", "error")).toBe(true);
    expect(matchesQuery("all good", "error")).toBe(false);
  });
});
