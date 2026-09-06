import { toGroundingLevel } from "../AssistantRepository";

test("passes through each known grounding level unchanged", () => {
  expect(toGroundingLevel("grounded")).toBe("grounded");
  expect(toGroundingLevel("mixed")).toBe("mixed");
  expect(toGroundingLevel("general")).toBe("general");
});

test("falls back to general for anything unrecognized, rather than crashing the UI", () => {
  expect(toGroundingLevel("")).toBe("general");
  expect(toGroundingLevel("unexpected-value")).toBe("general");
});
