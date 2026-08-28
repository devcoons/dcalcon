import { describe, expect, it } from "vitest";
import { buildRRule, parseRRule, rruleForSave, untilDateKey } from "./rrule";

describe("parseRRule", () => {
  it("returns empty fields when missing", () => {
    expect(parseRRule()).toEqual({ freq: "", interval: 1, until: "", count: "" });
    expect(parseRRule("")).toEqual({ freq: "", interval: 1, until: "", count: "" });
  });

  it("treats a missing INTERVAL as 1", () => {
    expect(parseRRule("FREQ=WEEKLY;BYDAY=MO,WE")).toMatchObject({ freq: "WEEKLY", interval: 1 });
  });

  it("parses UNTIL to a date field", () => {
    expect(parseRRule("FREQ=DAILY;INTERVAL=2;UNTIL=20261231T235959Z;COUNT=10")).toEqual({
      freq: "DAILY",
      interval: 2,
      until: "2026-12-31",
      count: "10",
    });
  });
});

describe("buildRRule", () => {
  it("omits INTERVAL when it is 1", () => {
    expect(buildRRule("WEEKLY", 1, "", "")).toBe("FREQ=WEEKLY");
  });

  it("returns empty when there is no FREQ", () => {
    expect(buildRRule("", 2, "2026-12-31", "3")).toBe("");
  });
});

describe("rruleForSave", () => {
  it("keeps the original string when FREQ, INTERVAL, COUNT, and UNTIL date match", () => {
    const original = "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,TH;WKST=MO;UNTIL=20261231T235959Z";
    expect(rruleForSave(original, "WEEKLY", 2, "2026-12-31", "")).toBe(original);
  });

  it("rebuilds when the operator changes FREQ", () => {
    expect(rruleForSave("FREQ=WEEKLY;BYDAY=MO", "DAILY", 1, "", "")).toBe("FREQ=DAILY");
  });
});

describe("untilDateKey", () => {
  it("compares date-only UNTIL values", () => {
    expect(untilDateKey("2026-08-28")).toBe("20260828");
    expect(untilDateKey("20260828T235959Z")).toBe("20260828");
  });
});
