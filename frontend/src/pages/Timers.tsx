import { useState } from "react";
import type { FormEvent } from "react";
import { api } from "../api";
import { TimerCard } from "../components/TimerCard";
import { useData } from "../data";
import { DEFAULT_DURATION_SECONDS, formatTimespan, parseTimespan } from "../timespan";

export function Timers() {
  const { state, act } = useData();
  const [name, setName] = useState("");
  const [dur, setDur] = useState("");
  const [bad, setBad] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!state) return <p className="muted">Loading…</p>;
  const titles = new Map(state.tasks.map((t) => [t.id, t.title]));

  const add = async (e: FormEvent) => {
    e.preventDefault();
    const r = dur.trim() ? parseTimespan(dur) : ({ ok: true, seconds: DEFAULT_DURATION_SECONDS } as const);
    setBad(!r.ok);
    setError(r.ok ? null : r.error);
    if (!r.ok) return;
    await act(() => api.createTimer(name.trim() || null, r.seconds));
    setName("");
  };

  return (
    <div>
      <form className="new-timer" onSubmit={add}>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Timer" maxLength={200} />
        <input
          className={bad ? "bad" : ""}
          value={dur}
          onChange={(e) => setDur(e.target.value)}
          placeholder={formatTimespan(DEFAULT_DURATION_SECONDS)}
          aria-label="Duration"
        />
        <button className="primary">Add timer</button>
        {error && <span className="preview bad-text">{error}</span>}
      </form>
      {state.timers.length === 0 ? (
        <p className="muted">No timers yet.</p>
      ) : (
        <div className="timer-grid">
          {state.timers.map((t) => (
            <TimerCard
              key={t.id}
              timer={t}
              label={t.task_id ? `Task: ${titles.get(t.task_id) ?? ""}` : (t.name ?? "Timer")}
              onDelete={t.task_id ? undefined : () => act(() => api.deleteTimer(t.id))}
            />
          ))}
        </div>
      )}
    </div>
  );
}
