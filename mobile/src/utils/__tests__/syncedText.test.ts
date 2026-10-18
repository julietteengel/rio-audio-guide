import { parseNarrationSegments, findActiveLineIndex } from "../syncedText";

describe("parseNarrationSegments", () => {
  it("parses one line per segment, converting seconds to milliseconds", () => {
    const json = JSON.stringify({
      segments: [
        { start: 0, text: " Au sommet du Corcovado." },
        { start: 3.5, text: "Le Christ Rédempteur domine Rio." },
      ],
    });
    expect(parseNarrationSegments(json)).toEqual([
      { time: 0, text: "Au sommet du Corcovado." },
      { time: 3500, text: "Le Christ Rédempteur domine Rio." },
    ]);
  });

  it("skips a segment missing start or text", () => {
    const json = JSON.stringify({
      segments: [
        { start: 0, text: "Au sommet." },
        { text: "pas de start" },
        { start: 5 },
      ],
    });
    expect(parseNarrationSegments(json)).toEqual([{ time: 0, text: "Au sommet." }]);
  });

  it("skips a segment whose text is blank once trimmed", () => {
    const json = JSON.stringify({ segments: [{ start: 0, text: "   " }] });
    expect(parseNarrationSegments(json)).toEqual([]);
  });

  it("returns an empty array for malformed JSON", () => {
    expect(parseNarrationSegments("not json at all")).toEqual([]);
  });

  it("returns an empty array when segments is missing", () => {
    expect(parseNarrationSegments(JSON.stringify({ text: "whole transcript" }))).toEqual([]);
  });

  it("returns an empty array for a JSON null body", () => {
    expect(parseNarrationSegments("null")).toEqual([]);
  });
});

describe("findActiveLineIndex", () => {
  const lines = [
    { time: 0, text: "Au sommet" },
    { time: 100, text: "du Corcovado" },
    { time: 250, text: "se dresse" },
  ];

  it("returns -1 before the first line", () => {
    expect(findActiveLineIndex(lines, -1)).toBe(-1);
  });

  it("returns the first index exactly at its own timestamp", () => {
    expect(findActiveLineIndex(lines, 0)).toBe(0);
  });

  it("returns the previous index between two timestamps", () => {
    expect(findActiveLineIndex(lines, 150)).toBe(1);
  });

  it("returns the last index once past the final timestamp", () => {
    expect(findActiveLineIndex(lines, 10000)).toBe(2);
  });

  it("returns -1 for an empty lines array", () => {
    expect(findActiveLineIndex([], 500)).toBe(-1);
  });
});
