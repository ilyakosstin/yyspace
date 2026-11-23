import { useState } from "react";
import { api } from "../api";
import { useData, useTick } from "../data";
import { clock } from "../format";
import { MAX_MINUTES } from "../timespan";
import type { Timer } from "../types";

interface Props {
  timer: Timer;
  label?: string;
  onDelete?: () => void;
  compact?: boolean;
}

export function useRemaining(timer: Timer): number {
  const { state, serverNow } = useData();
  useTick(timer.state === "RUNNING" ? 250 : 60_000);
  if (timer.state !== "RUNNING" || !state) return timer.remaining_ms;
  return Math.max(timer.remaining_ms - (serverNow() - Date.parse(state.server_time)), 0);
}

export function TimerCard({ timer, label, onDelete, compact }: Props) {
  const { act } = useData();
  const remaining = useRemaining(timer);
  const [editing, setEditing] = useState(false);
  const [hms, setHms] = useState({ h: "", m: "", s: "" });
  const [problem, setProblem] = useState<string | null>(null);

  const startEdit = () => {
    const total = Math.max(0, Math.ceil(remaining / 1000));
    setHms({
      h: String(Math.floor(total / 3600)),
      m: String(Math.floor((total % 3600) / 60)),
      s: String(total % 60),
    });
    setProblem(null);
    setEditing(true);
  };
  const commit = async () => {
    const seconds = Number(hms.h || 0) * 3600 + Number(hms.m || 0) * 60 + Number(hms.s || 0);
    if (seconds <= 0) return setProblem("The time must be longer than zero");
    if (seconds > MAX_MINUTES * 60) return setProblem("The longest allowed time is 7 days (168 hours)");
    setEditing(false);
    await act(() => api.setRemaining(timer.id, seconds));
  };
  const field = (k: "h" | "m" | "s", label: string) => (
    <label className="hms">
      <input
        autoFocus={k === "h"}
        inputMode="numeric"
        value={hms[k]}
        maxLength={k === "h" ? 3 : 2}
        onFocus={(e) => e.target.select()}
        onChange={(e) => {
          setHms({ ...hms, [k]: e.target.value.replace(/\D/g, "") });
          setProblem(null);
        }}
        aria-label={label}
      />
      {k}
    </label>
  );

  const run = (a: "start" | "pause" | "reset") => act(() => api.timerAction(timer.id, a));
  const { state } = timer;
  const progress = timer.duration_ms ? 1 - remaining / timer.duration_ms : 0;

  return (
    <div className={`timer ${state.toLowerCase()} ${compact ? "compact" : ""}`}>
      {label && <div className="timer-label">{label}</div>}
      <div className="timer-face">
        {editing ? (
          <div
            className="timer-edit"
            onKeyDown={(e) => {
              if (e.key === "Enter" && e.target instanceof HTMLInputElement) commit();
              if (e.key === "Escape") setEditing(false);
            }}
          >
            <div className="timer-edit-fields">
              {field("h", "Hours")}
              {field("m", "Minutes")}
              {field("s", "Seconds")}
            </div>
            <div className="timer-edit-actions">
              <button className="primary" onClick={commit}>Set</button>
              <button onClick={() => { setEditing(false); setProblem(null); }}>Cancel</button>
            </div>
          </div>
        ) : (
          <button className="timer-time" onClick={startEdit} aria-label={`Change remaining time (${clock(remaining)})`} title="Change remaining time">
            {clock(remaining)}
          </button>
        )}
        <div className="timer-buttons">
          {state === "RUNNING" ? (
            <button onClick={() => run("pause")}>Pause</button>
          ) : state === "FINISHED" ? (
            <span className="muted">Done</span>
          ) : (
            <button className="primary" onClick={() => run("start")}>
              {state === "PAUSED" ? "Resume" : "Start"}
            </button>
          )}
          <button onClick={() => run("reset")} disabled={state === "IDLE"}>
            Reset
          </button>
          {onDelete && (
            <button className="danger" onClick={onDelete} aria-label="Delete timer">
              ✕
            </button>
          )}
        </div>
      </div>
      {problem && <div className="preview bad-text">{problem}</div>}
      <div className="bar">
        <div style={{ width: `${Math.min(Math.max(progress, 0), 1) * 100}%` }} />
      </div>
    </div>
  );
}
