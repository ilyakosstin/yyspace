import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { api } from "./api";
import { open, ready } from "./crypto";
import type { AppState, Timer } from "./types";

async function openState(s: AppState): Promise<AppState> {
  const timer = async (t: Timer): Promise<Timer> => ({ ...t, name: await open(t.name) });
  return {
    ...s,
    tasks: await Promise.all(
      s.tasks.map(async (t) => ({
        ...t,
        title: await open(t.title),
        description: await open(t.description),
        timer: t.timer && (await timer(t.timer)),
      })),
    ),
    timers: await Promise.all(s.timers.map(timer)),
  };
}

interface Data {
  state: AppState | null;
  error: string | null;
  /** Estimated current server time (ms since epoch), corrected for client clock skew. */
  serverNow: () => number;
  /** Run a mutation, then reload the state. Errors are surfaced via `error`. */
  /** Reload from the server (e.g. after the encryption key changed). */
  refresh: () => Promise<void>;
  act: <T>(fn: () => Promise<T>) => Promise<T | undefined>;
}

const Ctx = createContext<Data | null>(null);
const POLL_MS = 20_000;

export function DataProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AppState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const receivedAt = useRef(Date.now());

  const refresh = useCallback(async () => {
    try {
      await ready;
      const s = await openState(await api.state());
      receivedAt.current = Date.now();
      setState(s);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Cannot reach the server");
    }
  }, []);

  const serverNow = useCallback(
    () => (state ? Date.parse(state.server_time) + (Date.now() - receivedAt.current) : Date.now()),
    [state],
  );

  const act = useCallback(
    async <T,>(fn: () => Promise<T>) => {
      try {
        const r = await fn();
        await refresh();
        return r;
      } catch (e) {
        setError(e instanceof Error ? e.message : "Something went wrong");
        await refresh();
        return undefined;
      }
    },
    [refresh],
  );

  // Initial load, periodic poll, and refresh when the tab becomes visible again.
  useEffect(() => {
    refresh();
    const id = setInterval(refresh, POLL_MS);
    const onVisible = () => document.visibilityState === "visible" && refresh();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(id);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refresh]);

  // Reload right when the next timer finishes / timeout passes, so the UI reflects the
  // server-side completion / cancellation immediately.
  useEffect(() => {
    if (!state) return;
    const now = serverNow();
    const due: number[] = [];
    for (const t of state.timers)
      if (t.state === "RUNNING") due.push(t.remaining_ms - (now - Date.parse(state.server_time)));
    for (const t of state.tasks)
      if (t.status === "TODO" && t.timeout_at) due.push(Date.parse(t.timeout_at) - now);
    if (!due.length) return;
    const wait = Math.max(Math.min(...due), 0) + 400;
    if (wait > POLL_MS) return;
    const id = setTimeout(refresh, wait);
    return () => clearTimeout(id);
  }, [state, serverNow, refresh]);

  const value = useMemo(() => ({ state, error, serverNow, act, refresh }), [state, error, serverNow, act, refresh]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useData(): Data {
  const v = useContext(Ctx);
  if (!v) throw new Error("DataProvider missing");
  return v;
}

/** Re-render every `ms` (default 1s) so countdowns tick. */
export function useTick(ms = 1000): void {
  const [, set] = useState(0);
  useEffect(() => {
    const id = setInterval(() => set((n) => n + 1), ms);
    return () => clearInterval(id);
  }, [ms]);
}
