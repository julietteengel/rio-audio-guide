import { plural } from "../plural";

describe("plural", () => {
  it("uses the singular form for exactly 1", () => {
    expect(plural("{count} lugar|lugares", 1)).toBe("1 lugar");
  });

  it("uses the plural form for 0 and for more than 1", () => {
    expect(plural("{count} lugar|lugares", 0)).toBe("0 lugares");
    expect(plural("{count} lugar|lugares", 17)).toBe("17 lugares");
  });

  it("only touches the word pair, leaving the rest of the template", () => {
    expect(plural("{count} lieu|lieux · {size}", 1)).toBe("1 lieu · {size}");
    expect(plural("{count} lieu|lieux · {size}", 2)).toBe("2 lieux · {size}");
  });

  it("leaves a template with no pair alone", () => {
    expect(plural("{count} items", 1)).toBe("1 items");
  });
});
