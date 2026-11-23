import { describe, expect, it } from "vitest";
import { formatTimespan, parseTimespan } from "./timespan";

const secs = (t: string) => {
  const r = parseTimespan(t);
  if (!r.ok) throw new Error(`"${t}" -> ${r.error}`);
  return r.seconds;
};
const err = (t: string) => {
  const r = parseTimespan(t);
  expect(r.ok, t).toBe(false);
  return r.ok ? "" : r.error;
};

describe("accepted", () => {
  it("the examples", () => {
    expect(secs("15 minutes")).toBe(15 * 60);
    expect(secs("1 hr 15 min")).toBe(75 * 60);
    expect(secs("1 hour")).toBe(3600);
    expect(secs("8 min")).toBe(8 * 60);
  });
  it("a bare number is minutes", () => expect(secs("25")).toBe(25 * 60));
  it("unit spellings", () => {
    for (const t of ["2h", "2 hr", "2 hrs", "2 hour", "2 hours"]) expect(secs(t), t).toBe(7200);
    for (const t of ["5m", "5 min", "5 mins", "5 minute", "5 minutes"]) expect(secs(t), t).toBe(300);
  });
  it("compact, separated and reordered", () => {
    expect(secs("1h15m")).toBe(75 * 60);
    expect(secs("2 hours and 30 minutes")).toBe(150 * 60);
    expect(secs("1 hr, 5 min")).toBe(65 * 60);
    expect(secs("15 min 1 hr")).toBe(75 * 60);
  });
  it("more than 59 minutes, case and spacing", () => {
    expect(secs("90 min")).toBe(90 * 60);
    expect(secs("  1   HR   15   MIN ")).toBe(75 * 60);
  });
  it("the 7 day maximum", () => expect(secs("168 hours")).toBe(7 * 24 * 3600));
});

describe("rejected", () => {
  it("seconds in any form", () => {
    for (const t of ["30 seconds", "45s", "1 min 30 sec", "1 hr 5 min 10 secs"])
      expect(err(t), t).toMatch(/seconds/i);
  });
  it("clock notation (it contains seconds)", () => {
    err("25:30");
    err("1:30:00");
  });
  it("decimals", () => expect(err("1.5 hours")).toMatch(/decimal/i));
  it("zero, empty and nonsense", () => {
    err("0 min");
    err("0");
    err("");
    err("abc");
    err("min");
    err("-5 min");
  });
  it("repeated units", () => {
    err("15 min 15 min");
    err("1 hr 2 hr");
  });
  it("unknown units and over the maximum", () => {
    err("1 fortnight");
    err("2 days");
    err("169 hours");
  });
});

describe("formatTimespan", () => {
  it("renders words and rounds up to minutes", () => {
    expect(formatTimespan(4500)).toBe("1 hr 15 min");
    expect(formatTimespan(3600)).toBe("1 hr");
    expect(formatTimespan(480)).toBe("8 min");
    expect(formatTimespan(1475)).toBe("25 min");
  });
});
