# Timing rules

Everything about how Focus Helper reads, stores and shows time. The deadline parser lives in
`frontend/src/deadline.ts` and its tests in `frontend/src/deadline.test.ts`. Every example below
is covered by those tests, using a "now" of **Tuesday 27 Oct 2026, 10:00**.

## 1. Typing a deadline

The deadline box on the main page (under **options**) takes a phrase instead of a calendar date.
A live preview shows the exact date it resolves to, or why it was rejected. An empty box means no
deadline (the placeholder reads "No deadline").

A phrase has up to three parts, in this order:

```
[until]  [day]  [time]        at least one of day / time is required
```

Matching is case-insensitive. Commas, extra spaces and the filler word `at` are ignored.
`P.M.` and `PM` are equal.

### 1.1 Day

| You type | Meaning | Example (now = Tue 27 Oct) |
|----------|---------|----------------------------|
| `today` | The current day | 27 Oct |
| `tomorrow` | The next day | 28 Oct |
| `friday` (or `fri`) | The next Friday, counting today if today is Friday | 30 Oct |
| `this friday` | Same as `friday` | 30 Oct |
| `next friday` | The first Friday **strictly after today** | 30 Oct |
| `tuesday` | Today (today is a Tuesday) | 27 Oct |
| `next tuesday` | A week from today, because today is already Tuesday | 3 Nov |

Weekday names: `monday mon`, `tuesday tue tues`, `wednesday wed`, `thursday thu thur thurs`,
`friday fri`, `saturday sat`, `sunday sun`.

"Next <weekday>" never means "this coming week's day after the next one". It is simply the
nearest such weekday after today. So on a Tuesday, `next friday` is the Friday three days away.

### 1.2 Time

| You type | Meaning |
|----------|---------|
| (nothing) | **23:59** on the chosen day |
| `15:50`, `15` | 15:50, 15:00. **24-hour clock when there is no AM/PM** |
| `3:50` | 03:50, not 15:50 |
| `3:50 PM`, `3pm`, `3:50pm` | 15:50, 15:00, 15:50 |
| `12am` / `12:30 am` | 00:00 / 00:30 |
| `12pm` | 12:00 |

Limits: 24-hour times run from `0:00` to `23:59`. With AM/PM the hour must be 1 to 12.
`24:00`, `25`, `13pm`, `0am` and `3:75` are rejected.

A time can stand alone, such as `17:30`, `5pm` or `at 17`, and then means today at that time.
A bare number with no day, no colon, no `at` and no AM/PM (such as `17`) is rejected as ambiguous.

### 1.3 `until`

`until` changes a **date without a time** to "before that day begins", which is 23:59 of the
previous day:

| You type | Deadline |
|----------|----------|
| `next tuesday` | 3 Nov, 23:59 |
| `until next tuesday` | **2 Nov**, 23:59 (the day before) |
| `until friday` | 29 Oct, 23:59 |
| `until tomorrow` | 27 Oct, 23:59 (today) |

If an explicit time is given, `until` has **no effect on the result**. `until tomorrow 3pm` and
`tomorrow 3pm` are both 28 Oct 15:00, and `until 5pm` is today 17:00. "Until 3pm" already means
"up to 3pm".

`until` on its own is rejected.

### 1.4 The past is not allowed

The resolved moment must be **strictly later than now**. Otherwise the phrase is rejected with
"That time is already in the past". Examples at 10:00: `today 9am`, `9:00` and `10:00` (exactly
now) are rejected, `today 11:00` is accepted, and `until today` is rejected because it means
yesterday 23:59. Nothing is rolled forward to the next day automatically.

The backend enforces the same rule: creating a task, or changing its deadline, with a deadline
that is not in the future returns 422.

### 1.5 Things that are not understood

`yesterday`, `next week`, `next month`, calendar dates such as `12 Nov`, and relative amounts such
as `in 2 hours`. These return an error. The parser never guesses.

## 2. Time zone and storage

- Phrases are interpreted in the **browser's local time zone**. "Today" is the user's today, and
  23:59 is local 23:59.
- The resolved moment is sent to the API as an absolute UTC timestamp. The server stores and
  returns UTC (ISO 8601 with `Z`). Timestamps without a time zone are rejected.
- Seconds are always 0, so `23:59` means 23:59:00.

## 3. Showing deadlines

- The to-do list is sorted by **nearest deadline first**. Tasks with no deadline go last, in
  creation order.
- A deadline is displayed only when it is **24 hours or less away**, or already **overdue**. It
  reads "due in 3h 12m" or "overdue 5m". Further-out deadlines still affect the order but are not
  shown.
- Overdue tasks stay in the to-do list. A deadline does not cancel anything.

## 4. Time constraint (CLASSIC tasks only)

The "time constraint" is how long a CLASSIC task may stay open. It is typed as a timespan
(section 5).

- The task is cancelled when that much time has passed since it was created.
- Left empty, the field means **no time constraint** (the grey placeholder says "None").
- TIMED tasks cannot have a time constraint. They take a **duration** instead, typed the same way.
  Left empty, it defaults to **25 minutes** (shown as the placeholder).
- A cancelled task records the exact moment its constraint ran out, even if nobody opened the app
  then. Expiry is applied on the next request.
- Restoring a task removes its time constraint.

## 5. Typing a timespan

Used for a task's time constraint, a TIMED task's duration, new timers, and changing the time left
on a timer. The parser is `frontend/src/timespan.ts`, with tests in `timespan.test.ts`. A live
preview shows the result, such as `→ 1 hr 15 min`.

**Defaults:** every field's grey placeholder shows the value used when you leave it empty. A new
timer defaults to 25 minutes, a timed task's duration to 25 minutes, a classic task's time
constraint to none, and a task's deadline to none.

| You type | Meaning |
|----------|---------|
| `15 minutes` | 15 min |
| `1 hr 15 min` | 1 h 15 min |
| `1 hour` | 1 h |
| `8 min` | 8 min |
| `25` | 25 min. **A lone number means minutes** |

Rules:

- **Seconds cannot be mentioned.** `30 seconds`, `45s`, `1 min 30 sec` are rejected, and so is
  clock notation such as `25:30` or `1:30:00`, because it contains seconds. Only whole minutes can be
  typed. (A running timer still counts down in seconds on screen.)
- Units: hours `h hr hrs hour hours`, minutes `m min mins minute minutes`. Case is ignored.
  Days and weeks are not understood, so write `48 hours`.
- Hours and minutes may be written together (`1h15m`), separated by spaces, commas, `+`, `&` or
  `and` (`2 hours and 30 minutes`), and in either order. Each unit may appear once.
- Minutes may exceed 59: `90 min` is 1 h 30 min.
- No decimals. Write `1 hr 30 min`, not `1.5 hours`.
- The total must be more than zero and at most **7 days (168 hours)**.
- When a timer's time is shown for editing, it is rounded **up** to the next whole minute, for
  example `24:35` appears as `25 min`. Leaving it untouched changes nothing.
- The API itself still takes seconds (`timeout_seconds`, `duration_seconds`, `remaining_seconds`).
  The "no seconds" rule applies to what you type in the UI.

## 6. Timers

- Remaining time is `duration − elapsed`. Elapsed time grows only while the timer is running.
- States: `IDLE` → `RUNNING` ⇄ `PAUSED` → `FINISHED`. **Reset** always returns to `IDLE` with the
  full duration.
- **Changing the time** edits the time *left*, in any state. A running timer keeps running, a
  paused one stays paused, and a finished one is reset first. The time is typed as a timespan
  (section 5).
- Timers are stored on the server and survive reloads and restarts. A timer that ran out while
  the app was closed is finished at its true end moment.
- A TIMED task is completed when its timer finishes, or by hand (for work done away from the app,
  which pauses the timer). Cancelling or completing a task pauses its
  timer, and resetting a finished TIMED task's timer reopens it.
- The browser counts down from the server's `remaining_ms` and does not compare its own clock to
  server timestamps, so a wrong local clock cannot change timer results.
