import { describe, expect, it } from "vitest";
import { addHourHHMM, icsMinutes, layoutDayTimed, minutesFromClientY, padHHMM, timedRange } from "./week-grid";

const day = new Date(2026, 7, 28);

describe("week grid time helpers", () => {
  it("snaps click minutes to 15-minute slots", () => {
    expect(padHHMM(9 * 60 + 7)).toBe("09:00");
    expect(padHHMM(9 * 60 + 8)).toBe("09:15");
    expect(padHHMM(-10)).toBe("00:00");
  });

  it("adds one hour without crossing midnight into the next day", () => {
    expect(addHourHHMM("09:00")).toBe("10:00");
    expect(addHourHHMM("23:30")).toBe("23:45");
  });

  it("reads minutes from compact ICS times", () => {
    expect(icsMinutes("20260828T093000")).toBe(9 * 60 + 30);
    expect(icsMinutes("20260828")).toBeNull();
  });

  it("maps a click on the timed canvas to minutes", () => {
    expect(minutesFromClientY(100, 100, 48 * 24)).toBe(0);
    expect(minutesFromClientY(100 + 48, 100, 48 * 24)).toBe(60);
  });
});

describe("layoutDayTimed", () => {
  it("ignores all-day events and tasks", () => {
    const blocks = layoutDayTimed(
      [
        { dtstart: "20260828", all_day: true },
        { dtstart: "20260828T090000", kind: "task" },
      ],
      day,
    );
    expect(blocks).toHaveLength(0);
  });

  it("places overlapping timed events in separate lanes", () => {
    const blocks = layoutDayTimed(
      [
        { dtstart: "20260828T090000", dtend: "20260828T110000" },
        { dtstart: "20260828T100000", dtend: "20260828T120000" },
      ],
      day,
    );
    expect(blocks).toHaveLength(2);
    expect(blocks.every((b) => b.cols === 2)).toBe(true);
    expect(new Set(blocks.map((b) => b.col)).size).toBe(2);
  });

  it("returns a timed range for events on that day", () => {
    const r = timedRange({ dtstart: "20260828T090000", dtend: "20260828T103000" }, day);
    expect(r).toEqual({ startMin: 9 * 60, endMin: 10 * 60 + 30 });
  });
});
