import { parseWordMarks, groupMarksIntoLines, findActiveLineIndex } from "../syncedText";

describe("parseWordMarks", () => {
  it("parses one word mark per line", () => {
    const ndjson = [
      '{"time":25,"type":"word","start":27,"end":29,"value":"Au"}',
      '{"time":136,"type":"word","start":30,"end":36,"value":"sommet"}',
    ].join("\n");
    expect(parseWordMarks(ndjson)).toEqual([
      { time: 25, value: "Au" },
      { time: 136, value: "sommet" },
    ]);
  });

  it("skips non-word mark types", () => {
    const ndjson = [
      '{"time":0,"type":"sentence","start":0,"end":10,"value":"Au sommet."}',
      '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}',
    ].join("\n");
    expect(parseWordMarks(ndjson)).toEqual([{ time: 25, value: "Au" }]);
  });

  it("skips a corrupted line without failing the whole file", () => {
    const ndjson = [
      '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}',
      "not json at all",
      '{"time":136,"type":"word","start":3,"end":9,"value":"sommet"}',
    ].join("\n");
    expect(parseWordMarks(ndjson)).toEqual([
      { time: 25, value: "Au" },
      { time: 136, value: "sommet" },
    ]);
  });

  it("skips a valid-JSON null line without throwing", () => {
    const ndjson = [
      '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}',
      "null",
      '{"time":136,"type":"word","start":3,"end":9,"value":"sommet"}',
    ].join("\n");
    expect(parseWordMarks(ndjson)).toEqual([
      { time: 25, value: "Au" },
      { time: 136, value: "sommet" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(parseWordMarks("")).toEqual([]);
  });

  it("ignores blank lines", () => {
    const ndjson = '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}\n\n';
    expect(parseWordMarks(ndjson)).toEqual([{ time: 25, value: "Au" }]);
  });
});

describe("groupMarksIntoLines", () => {
  it("groups words until adding the next would exceed the target length", () => {
    const marks = [
      { time: 0, value: "Au" },
      { time: 100, value: "sommet" },
      { time: 200, value: "du" },
      { time: 300, value: "Corcovado" },
    ];
    // Target 10: "Au sommet" is 9 chars (fits); "Au sommet du" would be 12
    // (flush, 2+ words already held) -- "du" alone is under the minimum of
    // 2 words, so it's forced together with "Corcovado" even though that
    // pair is also over the target.
    expect(groupMarksIntoLines(marks, 10)).toEqual([
      { time: 0, text: "Au sommet" },
      { time: 200, text: "du Corcovado" },
    ]);
  });

  it("does not split when the combined length exactly equals the target", () => {
    const marks = [
      { time: 0, value: "Au" },
      { time: 100, value: "sommet" },
    ];
    expect(groupMarksIntoLines(marks, 9)).toEqual([{ time: 0, text: "Au sommet" }]);
  });

  it("keeps a single trailing word alone when there is nothing left to combine it with", () => {
    expect(groupMarksIntoLines([{ time: 0, value: "Au" }], 10)).toEqual([
      { time: 0, text: "Au" },
    ]);
  });

  it("returns an empty array for empty input", () => {
    expect(groupMarksIntoLines([], 40)).toEqual([]);
  });

  it("defaults the target to 40 characters", () => {
    const marks = [
      { time: 0, value: "El" },
      { time: 50, value: "Cristo" },
      { time: 100, value: "Redentor" },
      { time: 150, value: "mira" },
      { time: 200, value: "hacia" },
      { time: 250, value: "la" },
      { time: 300, value: "bahía" },
      { time: 350, value: "de" },
      { time: 400, value: "Guanabara" },
    ];
    // "El Cristo Redentor mira hacia la bahía de" is 42 chars -- over 40,
    // so it flushes before "de", not after.
    expect(groupMarksIntoLines(marks)).toEqual([
      { time: 0, text: "El Cristo Redentor mira hacia la bahía" },
      { time: 350, text: "de Guanabara" },
    ]);
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
