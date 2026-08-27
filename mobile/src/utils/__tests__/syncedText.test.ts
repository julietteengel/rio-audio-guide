import { parseWordMarks, findActiveWordIndex } from "../syncedText";

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

  it("returns an empty array for empty input", () => {
    expect(parseWordMarks("")).toEqual([]);
  });

  it("ignores blank lines", () => {
    const ndjson = '{"time":25,"type":"word","start":0,"end":2,"value":"Au"}\n\n';
    expect(parseWordMarks(ndjson)).toEqual([{ time: 25, value: "Au" }]);
  });
});

describe("findActiveWordIndex", () => {
  const marks = [
    { time: 0, value: "Au" },
    { time: 100, value: "sommet" },
    { time: 250, value: "du" },
  ];

  it("returns -1 before the first mark", () => {
    expect(findActiveWordIndex(marks, -1)).toBe(-1);
  });

  it("returns the first index exactly at its own timestamp", () => {
    expect(findActiveWordIndex(marks, 0)).toBe(0);
  });

  it("returns the previous index between two timestamps", () => {
    expect(findActiveWordIndex(marks, 150)).toBe(1);
  });

  it("returns the last index once past the final timestamp", () => {
    expect(findActiveWordIndex(marks, 10000)).toBe(2);
  });

  it("returns -1 for an empty marks array", () => {
    expect(findActiveWordIndex([], 500)).toBe(-1);
  });
});
