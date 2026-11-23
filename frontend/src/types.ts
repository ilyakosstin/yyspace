export type TaskType = "CLASSIC" | "TIMED";
export type TaskStatus = "TODO" | "COMPLETED" | "CANCELLED";
export type TimerState = "IDLE" | "RUNNING" | "PAUSED" | "FINISHED";

export interface Timer {
  id: number;
  task_id: number | null;
  name: string | null;
  state: TimerState;
  duration_ms: number;
  remaining_ms: number; // as of server_time
}

export interface Task {
  id: number;
  title: string;
  description: string | null;
  type: TaskType;
  status: TaskStatus;
  deadline: string | null;
  timeout_at: string | null;
  created_at: string;
  finished_at: string | null;
  timer: Timer | null;
}

export interface TaskPatch {
  title?: string;
  description?: string | null;
  deadline?: string | null;
  timeout_seconds?: number | null;
}

export interface AppState {
  server_time: string;
  tasks: Task[];
  timers: Timer[];
}

export interface NewTask {
  title: string;
  description?: string | null;
  type: TaskType;
  deadline?: string | null;
  timeout_seconds?: number | null;
  duration_seconds?: number | null;
}
