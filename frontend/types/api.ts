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

export type GameResult = "W" | "L" | "D" | "C";

export interface SetScore {
  our_score: number;
  opponent_score: number;
}

export interface Game {
  round?: string;
  opponent_name: string;
  our_team_name: string;
  team_rgx: number;
  opponent_rgx: number;
  result: GameResult;
  rgx_delta: number;
  rgx_delta_per_player: number;
  performance_delta: number;
  performance_delta_per_player: number;
  has_proxy: boolean;
  was_cancelled: boolean;
  sets: SetScore[];
  // A woman's league Open Partie: it counts towards her Open RGX, not the Women RGX shown.
  open_rating?: boolean;
}

export interface TodayResultOk {
  status: "ok";
  // Set when the games are Partien of a league matchday, omitted for a tournament.
  competition?: "league";
  stale?: boolean;
  player: PlayerRef;
  date: string;
  // Omitted by the backend (Go `omitempty`) when the player is in an active
  // tournament but hasn't played a game yet today — treat as 0/false/[].
  total_delta?: number;
  total_performance_delta?: number;
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

// GET /api/stats/rgx-jobs — all-time RgxJob usage counts.
export interface RgxJobStats {
  created: number;
  succeeded: number;
  failed: number;
}

// GET /api/tournaments/upcoming — soonest first. Dates are YYYY-MM-DD.
export interface UpcomingTournament {
  id: number;
  name: string;
  location: string;
  start_date: string;
  end_date: string;
}

// POST /api/feedback
export interface FeedbackRequest {
  rating: number;
  text?: string;
}

export interface FeedbackResponse {
  status: "ok";
  id: number;
}
