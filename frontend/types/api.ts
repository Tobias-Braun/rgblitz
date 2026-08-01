// Mirrors the Go backend's response shapes exactly — see docs/06-api-spec.md.
// Keep in sync with backend/internal/api/handlers.go if the contract changes.

export interface PlayerSummary {
  id: string;
  name: string;
  club_name: string;
  avatar_url: string;
  club_logo_url: string;
  rgx: number;
}

export interface PlayerRef {
  id: string;
  name: string;
  club_name: string;
  avatar_url: string;
  club_logo_url: string;
  rgx: number;
}

export type GameResult = "W" | "L" | "D";

export interface Game {
  round?: string;
  opponent_name: string;
  result: GameResult;
  rgx_delta: number;
  has_proxy: boolean;
  // Not yet in docs/06-api-spec.md — render only when the backend supplies them.
  format?: string;
  sets?: ("W" | "L")[];
}

export interface TodayResultOk {
  status: "ok";
  stale?: boolean;
  player: PlayerRef;
  date: string;
  // Omitted by the backend (Go `omitempty`) when the player is in an active
  // tournament but hasn't played a game yet today — treat as 0/false/[].
  total_delta?: number;
  has_proxy?: boolean;
  games?: Game[];
}

export interface TodayResultNoTournament {
  status: "no_tournament";
  player: PlayerRef;
}

export type TodayResult = TodayResultOk | TodayResultNoTournament;

export interface ApiError {
  error: string;
  stale?: boolean;
}

// GET /api/players/:id/today (get-or-create a job) and
// GET /api/players/:id/today/jobs/:jobId (poll a job's status) both return
// this shape. "queued"/"running" are non-final — keep polling the jobs
// endpoint until the status becomes "done", "error", or "no_tournament".
export type JobStatus = "no_tournament" | "queued" | "running" | "done" | "error";

export interface JobResponse {
  job_id?: number;
  status: JobStatus;
  queue_position?: number;
  result?: TodayResult;
  error?: string;
  finished_at?: string;
}
