import { describe, expect, it } from "vitest";
import { parseDeadline } from "./deadline";

// Tuesday, 27 Oct 2026, 10:00 local time.
const NOW = new Date(2026, 9, 27, 10, 0);

function ok(text: string, now = NOW): string {
  const r = parseDeadline(text, now);
  if (!r.ok) throw new Error(`"${text}" -> ${r.error}`);
  const d = r.date;
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}
const err = (text: string, now = NOW) => {
  const r = parseDeadline(text, now);
  expect(r.ok, text).toBe(false);
  return r.ok ? "" : r.error;
};

describe("day words", () => {
  it("today defaults to 23:59", () => expect(ok("today")).toBe("2026-10-27 23:59"));
  it("tomorrow with a PM time", () => expect(ok("tomorrow 3:50 PM")).toBe("2026-10-28 15:50"));
  it("next tuesday on a Tuesday is a week away", () => expect(ok("next tuesday")).toBe("2026-11-03 23:59"));
  it("bare weekday on that weekday means today", () => expect(ok("tuesday")).toBe("2026-10-27 23:59"));
  it("bare / this / next weekday agree when it is later this week", () => {
    expect(ok("friday")).toBe("2026-10-30 23:59");
    expect(ok("this friday")).toBe("2026-10-30 23:59");
    expect(ok("next friday")).toBe("2026-10-30 23:59");
  });
  it("weekday abbreviations", () => {
    expect(ok("tues 18:00")).toBe("2026-10-27 18:00");
    expect(ok("thurs")).toBe("2026-10-29 23:59");
  });
  it("rolls over month ends", () => expect(ok("tomorrow", new Date(2026, 9, 31, 8, 0))).toBe("2026-11-01 23:59"));
});

describe("until", () => {
  it("until <date> means the end of the day before", () => {
    expect(ok("until next tuesday")).toBe("2026-11-02 23:59");
    expect(ok("until friday")).toBe("2026-10-29 23:59");
    expect(ok("until tomorrow")).toBe("2026-10-27 23:59");
  });
  it("until <date> <time> is the same instant as without until", () => {
    expect(ok("until tomorrow 3pm")).toBe("2026-10-28 15:00");
    expect(ok("until next tuesday 9:00")).toBe("2026-11-03 09:00");
  });
  it("until today is already past", () => expect(err("until today")).toMatch(/past/));
  it("until alone is rejected", () => err("until"));
});

describe("times", () => {
  it("is 24-hour without AM/PM", () => {
    expect(ok("tomorrow 3")).toBe("2026-10-28 03:00");
    expect(ok("tomorrow 3:50")).toBe("2026-10-28 03:50");
    expect(ok("tomorrow 15:50")).toBe("2026-10-28 15:50");
    expect(ok("tomorrow 0:00")).toBe("2026-10-28 00:00");
  });
  it("handles 12 AM / 12 PM", () => {
    expect(ok("tomorrow 12am")).toBe("2026-10-28 00:00");
    expect(ok("tomorrow 12:30 am")).toBe("2026-10-28 00:30");
    expect(ok("tomorrow 12pm")).toBe("2026-10-28 12:00");
  });
  it("accepts a time on its own when it is marked", () => {
    expect(ok("17:30")).toBe("2026-10-27 17:30");
    expect(ok("5pm")).toBe("2026-10-27 17:00");
    expect(ok("at 17")).toBe("2026-10-27 17:00");
    expect(ok("until 5pm")).toBe("2026-10-27 17:00");
  });
  it("rejects a bare number on its own", () => err("17"));
  it("rejects invalid clock values", () => {
    err("tomorrow 24:00");
    err("tomorrow 25");
    err("tomorrow 13pm");
    err("tomorrow 0am");
    err("tomorrow 3:75");
  });
});

describe("the past", () => {
  it("rejects times earlier today", () => {
    expect(err("today 9am")).toMatch(/past/);
    expect(err("9:00")).toMatch(/past/);
    expect(err("10:00")).toMatch(/past/); // exactly now
  });
  it("accepts later today", () => expect(ok("today 11:00")).toBe("2026-10-27 11:00"));
});

describe("input handling", () => {
  it("ignores case, commas and extra spaces", () =>
    expect(ok("  TOMORROW,   3:50   P.M. ")).toBe("2026-10-28 15:50"));
  it("rejects gibberish and empty input", () => {
    err("someday");
    err("yesterday");
    err("next month");
    err("");
  });
});
