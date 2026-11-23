import { seal } from "./crypto";
import type { AppState, NewTask, Task, TaskPatch, Timer } from "./types";

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const j = await res.json();
      detail = typeof j.detail === "string" ? j.detail : "Invalid input";
    } catch {
      /* non-JSON error body */
    }
    throw new Error(detail);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

export const api = {
  state: () => call<AppState>("GET", "/state"),
  createTask: async (t: NewTask) =>
    call<Task>("POST", "/tasks", { ...t, title: await seal(t.title), description: await seal(t.description) }),
  updateTask: async (id: number, patch: TaskPatch) => {
    const body = { ...patch };
    if (body.title) body.title = await seal(body.title);
    if (body.description) body.description = await seal(body.description);
    return call<Task>("PATCH", `/tasks/${id}`, body);
  },
  taskAction: (id: number, action: "complete" | "cancel" | "reopen") =>
    call<Task>("POST", `/tasks/${id}/${action}`),
  deleteTask: (id: number) => call<void>("DELETE", `/tasks/${id}`),
  createTimer: async (name: string | null, duration_seconds: number) =>
    call<Timer>("POST", "/timers", { name: await seal(name), duration_seconds }),
  timerAction: (id: number, action: "start" | "pause" | "reset") =>
    call<Timer>("POST", `/timers/${id}/${action}`),
  setRemaining: (id: number, remaining_seconds: number) =>
    call<Timer>("PATCH", `/timers/${id}`, { remaining_seconds }),
  deleteTimer: (id: number) => call<void>("DELETE", `/timers/${id}`),
};
