/**
 * Natural-language deadline parser. Pure and timezone-local (the browser's), so
 * "today" means the user's today. Rules are documented in TIMING_RULES.md.
 */

export type ParsedDeadline = { ok: true; date: Date } | { ok: false; error: string };

const WEEKDAYS: Record<string, number> = {
  sunday: 0, sun: 0,
  monday: 1, mon: 1,
  tuesday: 2, tue: 2, tues: 2,
  wednesday: 3, wed: 3,
  thursday: 4, thu: 4, thur: 4, thurs: 4,
  friday: 5, fri: 5,
  saturday: 6, sat: 6,
};

const DAY_RE = "today|tomorrow|(?:(?:next|this)\\s+)?(?:" + Object.keys(WEEKDAYS).join("|") + ")";
const TIME_RE = "(\\d{1,2})(?::(\\d{2}))?\\s*(am|pm)?";
const PATTERN = new RegExp(`^(until\\s+)?(?:(${DAY_RE})\\b\\s*)?(?:(at\\s+)?${TIME_RE})?$`);

const fail = (error: string): ParsedDeadline => ({ ok: false, error });

export function parseDeadline(input: string, now: Date = new Date()): ParsedDeadline {
  const text = input
    .toLowerCase()
    .replace(/[,.]/g, " ")
    .replace(/\b([ap]) m\b/g, "$1m")
    .replace(/\s+/g, " ")
    .trim();
  if (!text) return fail("Type a deadline, e.g. “tomorrow 3:50 PM”");

  const m = PATTERN.exec(text);
  if (!m) return fail("Couldn’t understand that. Try “today”, “tomorrow 15:30” or “until friday”");
  const [, until, day, at, hh, mm, meridiem] = m;
  const hasTime = hh !== undefined;
  if (!day && !hasTime) return fail("Add a day or a time after “until”");

  // A time on its own needs a marker, otherwise a bare number is too ambiguous.
  if (!day && hasTime && !(at || mm !== undefined || meridiem))
    return fail("Write the time as 17:30, 5pm or “at 17”");

  let hour = 0;
  let minute = 0;
  if (hasTime) {
    hour = Number(hh);
    minute = mm === undefined ? 0 : Number(mm);
    if (minute > 59) return fail(`${mm} is not a valid minute`);
    if (meridiem) {
      if (hour < 1 || hour > 12) return fail(`${hh} ${meridiem.toUpperCase()} is not a valid time`);
      hour = (hour % 12) + (meridiem === "pm" ? 12 : 0);
    } else if (hour > 23) {
      return fail(`${hh}:${String(minute).padStart(2, "0")} is not a valid 24-hour time`);
    }
  } else {
    hour = 23;
    minute = 59;
  }

  // Resolve the calendar day.
  const date = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  if (day === "tomorrow") {
    date.setDate(date.getDate() + 1);
  } else if (day && day !== "today") {
    const parts = day.split(" ");
    const target = WEEKDAYS[parts[parts.length - 1]];
    let diff = (target - date.getDay() + 7) % 7;
    if (diff === 0 && parts[0] === "next") diff = 7;
    date.setDate(date.getDate() + diff);
  }

  // "until <date>" without a time means "before that day starts".
  if (until && day && !hasTime) {
    date.setDate(date.getDate() - 1);
  }

  date.setHours(hour, minute, 0, 0);
  if (date.getTime() <= now.getTime()) return fail("That time is already in the past");
  return { ok: true, date };
}
