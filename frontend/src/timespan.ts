/**
 * Natural-language timespan parser ("15 minutes", "1 hr 15 min"). Whole minutes only:
 * seconds are deliberately not accepted. Rules are documented in TIMING_RULES.md.
 */

export type ParsedTimespan = { ok: true; seconds: number } | { ok: false; error: string };

export const MAX_MINUTES = 7 * 24 * 60;

/** Used when a duration field is left empty (timed tasks, new timers). */
export const DEFAULT_DURATION_SECONDS = 25 * 60;

const HOURS = new Set(["h", "hr", "hrs", "hour", "hours"]);
const MINUTES = new Set(["m", "min", "mins", "minute", "minutes"]);
const SECONDS = new Set(["s", "sec", "secs", "second", "seconds"]);

const fail = (error: string): ParsedTimespan => ({ ok: false, error });
const EXAMPLES = "Try “15 minutes” or “1 hr 15 min”";

export function parseTimespan(input: string): ParsedTimespan {
  const text = input
    .toLowerCase()
    .replace(/[,&+]|\band\b/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  if (!text) return fail(EXAMPLES);
  if (text.includes(":")) return fail("Write it in words, like “1 hr 15 min”");

  // A lone number means minutes.
  if (/^\d+$/.test(text)) return finish(0, Number(text));

  let hours: number | null = null;
  let minutes: number | null = null;
  const token = /(\d+)\s*([a-z]+)\s*/y;
  let pos = 0;
  while (pos < text.length) {
    token.lastIndex = pos;
    const m = token.exec(text);
    if (!m) return fail(/\d\.\d/.test(text) ? "No decimals. Use “1 hr 30 min”" : `Couldn’t understand that. ${EXAMPLES}`);
    pos = token.lastIndex;
    const [, num, unit] = m;
    if (SECONDS.has(unit)) return fail("Seconds aren’t allowed. Use whole minutes");
    if (HOURS.has(unit)) {
      if (hours !== null) return fail("Hours are mentioned twice");
      hours = Number(num);
    } else if (MINUTES.has(unit)) {
      if (minutes !== null) return fail("Minutes are mentioned twice");
      minutes = Number(num);
    } else {
      return fail(`“${unit}” isn’t a unit I know. Use hours or minutes`);
    }
  }
  return finish(hours ?? 0, minutes ?? 0);
}

function finish(hours: number, minutes: number): ParsedTimespan {
  const total = hours * 60 + minutes;
  if (total <= 0) return fail("The time must be longer than zero");
  if (total > MAX_MINUTES) return fail("The longest allowed time is 7 days (168 hours)");
  return { ok: true, seconds: total * 60 };
}

/** 4500 -> "1 hr 15 min". Rounds up to whole minutes. */
export function formatTimespan(seconds: number): string {
  const total = Math.max(1, Math.ceil(seconds / 60));
  const h = Math.floor(total / 60);
  const m = total % 60;
  return [h ? `${h} hr` : "", m ? `${m} min` : ""].filter(Boolean).join(" ");
}
