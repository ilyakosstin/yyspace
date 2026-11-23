import { useState } from "react";
import type { FormEvent } from "react";
import Markdown from "react-markdown";
import { api } from "../api";
import { parseDeadline } from "../deadline";
import { useData, useTick } from "../data";
import { DAY_MS, span } from "../format";
import { formatTimespan, parseTimespan } from "../timespan";
import type { Task, TaskPatch } from "../types";
import { TimerCard } from "./TimerCard";

function TaskEditor({ task, onClose }: { task: Task; onClose: () => void }) {
  const { act } = useData();
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description ?? "");
  const [deadline, setDeadline] = useState("");
  const [timeout, setTimeout_] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const todo = task.status === "TODO";
  // Empty = keep as is; "none" removes.
  const clearing = (v: string) => v.trim().toLowerCase() === "none";

  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return setProblem("The title can’t be empty");
    const patch: TaskPatch = {};
    if (title.trim() !== task.title) patch.title = title;
    if (description.trim() !== (task.description ?? "")) patch.description = description.trim() || null;
    if (deadline.trim()) {
      if (clearing(deadline)) patch.deadline = null;
      else {
        const d = parseDeadline(deadline);
        if (!d.ok) return setProblem(d.error);
        patch.deadline = d.date.toISOString();
      }
    }
    if (timeout.trim()) {
      if (clearing(timeout)) patch.timeout_seconds = null;
      else {
        const t = parseTimespan(timeout);
        if (!t.ok) return setProblem(t.error);
        patch.timeout_seconds = t.seconds;
      }
    }
    if (Object.keys(patch).length) await act(() => api.updateTask(task.id, patch));
    onClose();
  };

  return (
    <form className="task-edit" onSubmit={save} onKeyDown={(e) => e.key === "Escape" && onClose()}>
      <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={500} aria-label="Title" autoFocus />
      <label>
        Description <span className="hint">markdown</span>
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} maxLength={20000} />
      </label>
      {todo && (
        <label>
          Deadline <span className="hint">e.g. tomorrow 3:50 PM · “none” removes it</span>
          <input
            value={deadline}
            onChange={(e) => setDeadline(e.target.value)}
            placeholder={task.deadline ? `Now: ${new Date(task.deadline).toLocaleString()}` : "No deadline"}
          />
        </label>
      )}
      {todo && task.type === "CLASSIC" && (
        <label>
          Time constraint <span className="hint">counted from now, e.g. 1 hr 15 min · “none” removes it</span>
          <input
            value={timeout}
            onChange={(e) => setTimeout_(e.target.value)}
            placeholder={
              task.timeout_at
                ? `Now: cancels in ${formatTimespan((Date.parse(task.timeout_at) - Date.now()) / 1000)}`
                : "None"
            }
          />
        </label>
      )}
      {problem && <span className="preview bad-text">{problem}</span>}
      <div className="row">
        <button className="primary">Save</button>
        <button type="button" onClick={onClose}>
          Cancel
        </button>
      </div>
    </form>
  );
}

export function TaskRow({ task }: { task: Task }) {
  const { act, serverNow } = useData();
  const [editing, setEditing] = useState(false);
  useTick(task.status === "TODO" ? 1000 : 60_000);
  const todo = task.status === "TODO";
  const now = serverNow();

  // Deadlines are only shown when they are near: within a day (or already overdue).
  let deadline: { text: string; late: boolean } | null = null;
  if (task.deadline && todo) {
    const left = Date.parse(task.deadline) - now;
    if (left <= DAY_MS)
      deadline = left < 0 ? { text: `overdue ${span(-left)}`, late: true } : { text: `due in ${span(left)}`, late: false };
  }
  const timeoutLeft = todo && task.timeout_at ? Date.parse(task.timeout_at) - now : null;

  if (editing)
    return (
      <li className={`task ${task.status.toLowerCase()}`}>
        <TaskEditor task={task} onClose={() => setEditing(false)} />
      </li>
    );

  return (
    <li className={`task ${task.status.toLowerCase()}`}>
      <div className="task-main">
        <span className="task-title">{task.title}</span>
        <span className="meta">
          {deadline && <span className={deadline.late ? "late" : "soon"}>{deadline.text}</span>}
          {timeoutLeft !== null && <span className="muted">cancels in {span(timeoutLeft)}</span>}
          {!todo && task.finished_at && (
            <span className="muted">{new Date(task.finished_at).toLocaleString()}</span>
          )}
        </span>
        <span className="actions">
          <button aria-label="Edit task" onClick={() => setEditing(true)}>
            Edit
          </button>
          {todo && (
            <button className="primary" onClick={() => act(() => api.taskAction(task.id, "complete"))}>
              Done
            </button>
          )}
          {todo && <button onClick={() => act(() => api.taskAction(task.id, "cancel"))}>Cancel</button>}
          {!todo && <button onClick={() => act(() => api.taskAction(task.id, "reopen"))}>Restore</button>}
          <button className="danger" aria-label="Delete task" onClick={() => act(() => api.deleteTask(task.id))}>
            ✕
          </button>
        </span>
        {task.description && (
          <div className="desc">
            <Markdown>{task.description}</Markdown>
          </div>
        )}
      </div>
      {todo && task.timer && <TimerCard timer={task.timer} compact />}
    </li>
  );
}
