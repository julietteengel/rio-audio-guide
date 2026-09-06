import { formatDuration } from "../itineraryFormat";

test("formats sub-hour durations as plain minutes", () => {
  expect(formatDuration(0)).toBe("0 min");
  expect(formatDuration(45)).toBe("45 min");
});

test("formats hour-plus durations as Nh + remaining minutes", () => {
  expect(formatDuration(60)).toBe("1h");
  expect(formatDuration(90)).toBe("1h30");
  expect(formatDuration(125)).toBe("2h05");
});
