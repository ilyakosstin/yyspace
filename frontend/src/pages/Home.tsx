import { useMemo, useState } from "react";
import type { FormEvent } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";
import { parseDeadline } from "../deadline";
import { useData } from "../data";
import { DEFAULT_DURATION_SECONDS, formatTimespan, parseTimespan } from "../timespan";
import type { NewTask, TaskType } from "../types";

export function Home() {
  const { state, act } = useData();
  const [title, setTitle] = useState("");
  const [more, setMore] = useState(false);
  const [type, setType] = useState<TaskType>("CLASSIC");
  const [deadline, setDeadline] = useState("");
  const [description, setDescription] = useState("");
  const [span, setSpan] = useState(""); // time constraint (classic) or duration (timed)
  const [note, setNote] = useState<string | null>(null);

  const parsedSpan = useMemo(() => (span.trim() ? parseTimespan(span) : null), [span]);
  const parsed = useMemo(() => (deadline.trim() ? parseDeadline(deadline) : null), [deadline]);

  const open = state?.tasks.filter((t) => t.status === "TODO").length ?? 0;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    if (parsed && !parsed.ok) return setNote(parsed.error);
    if (parsedSpan && !parsedSpan.ok) return setNote(parsedSpan.error);
    // Empty fields fall back to their defaults: no deadline, no time constraint, 25 min duration.
    const secs = parsedSpan?.ok ? parsedSpan.seconds : null;

    const body: NewTask = { title, type };
    if (description.trim()) body.description = description;
    if (parsed?.ok) body.deadline = parsed.date.toISOString();
    if (type === "TIMED") body.duration_seconds = secs ?? DEFAULT_DURATION_SECONDS;
    else if (secs) body.timeout_seconds = secs;

    const created = await act(() => api.createTask(body));
    if (created) {
      setTitle("");
      setSpan("");
      setDescription("");
      setDeadline("");
      setNote("Scheduled");
      setTimeout(() => setNote(null), 1800);
    }
  };

  return (
    <div className="home">
      <form onSubmit={submit}>
        <input
          className="big-input"
          enterKeyHint="done"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="What needs doing?"
          maxLength={500}
          aria-label="New task"
        />
        <div className="home-sub">
          <button type="button" className="link" onClick={() => setMore(!more)}>
            {more ? "less" : "advanced"}
          </button>
          {note ? (
            <span role="status" className={note === "Scheduled" ? "muted" : "bad-text"}>{note}</span>
          ) : (
            <span className="muted enter-hint">press Enter to schedule</span>
          )}
        </div>
        {more && (
          <div className="options">
            <div className="seg">
              <button type="button" className={type === "CLASSIC" ? "on" : ""} onClick={() => setType("CLASSIC")}>
                Classic
              </button>
              <button type="button" className={type === "TIMED" ? "on" : ""} onClick={() => setType("TIMED")}>
                Timed
              </button>
            </div>
            <label>
              {type === "TIMED" ? "Duration" : "Time constraint"}
              <span className="hint">e.g. 15 minutes, 1 hr 15 min</span>
              <input
                value={span}
                onChange={(e) => setSpan(e.target.value)}
                placeholder={type === "TIMED" ? formatTimespan(DEFAULT_DURATION_SECONDS) : "None"}
                className={parsedSpan && !parsedSpan.ok ? "bad" : ""}
              />
              {parsedSpan && (
                <span className={parsedSpan.ok ? "preview" : "preview bad-text"}>
                  {parsedSpan.ok ? `→ ${formatTimespan(parsedSpan.seconds)}` : parsedSpan.error}
                </span>
              )}
            </label>
            <label>
              Description
              <span className="hint">markdown</span>
              <textarea value={description} onChange={(e) => setDescription(e.target.value)} maxLength={20000} />
            </label>
            <label>
              Deadline
              <span className="hint">e.g. tomorrow 3:50 PM, until friday</span>
              <input
                value={deadline}
                onChange={(e) => setDeadline(e.target.value)}
                placeholder="No deadline"
                className={parsed && !parsed.ok ? "bad" : ""}
              />
              {parsed && (
                <span className={parsed.ok ? "preview" : "preview bad-text"}>
                  {parsed.ok ? `→ ${parsed.date.toLocaleString([], { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}` : parsed.error}
                </span>
              )}
            </label>
          </div>
        )}
        <button type="submit" className="primary add-task" disabled={!title.trim()}>
          Add task
        </button>
      </form>
      <Link to="/tasks" className="muted open-link">
        {open} open {open === 1 ? "task" : "tasks"} →
      </Link>
    </div>
  );
}
