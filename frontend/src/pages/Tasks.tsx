import { useState } from "react";
import { TaskRow } from "../components/TaskRow";
import { useData } from "../data";
import type { TaskStatus } from "../types";

const TABS: { status: TaskStatus; label: string; empty: string }[] = [
  { status: "TODO", label: "To do", empty: "Nothing to do. Enjoy it." },
  { status: "COMPLETED", label: "Completed", empty: "Nothing completed yet." },
  { status: "CANCELLED", label: "Cancelled", empty: "Nothing cancelled." },
];

export function Tasks() {
  const { state } = useData();
  const [tab, setTab] = useState<TaskStatus>("TODO");
  if (!state) return <p className="muted">Loading…</p>;

  const current = TABS.find((t) => t.status === tab)!;
  const list = state.tasks.filter((t) => t.status === tab);
  return (
    <div>
      <div className="tabs" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.status}
            role="tab"
            aria-selected={tab === t.status}
            className={tab === t.status ? "on" : ""}
            onClick={() => setTab(t.status)}
          >
            {t.label} <span className="count">{state.tasks.filter((x) => x.status === t.status).length}</span>
          </button>
        ))}
      </div>
      {list.length === 0 ? (
        <p className="muted">{current.empty}</p>
      ) : (
        <ul className="tasks">
          {list.map((t) => (
            <TaskRow key={t.id} task={t} />
          ))}
        </ul>
      )}
    </div>
  );
}
