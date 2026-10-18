import type { TodayResult } from "~/types/api";

export type ResultState = "ok" | "stale" | "no_games" | "no_tournament" | "error";

// Mirrors the v-if chain in pages/index.vue: once a job has settled, a missing
// result always means the error view.
export function resultState(data: TodayResult | null): ResultState {
  if (!data) return "error";
  if (data.status === "no_tournament") return "no_tournament";
  if (data.stale) return "stale";
  if (!data.games?.length) return "no_games";
  return "ok";
}

// Coarse 10-point buckets [lower, upper) keep the reported delta from
// identifying a single player's exact result.
export function deltaBucket(delta: number): string {
  if (delta === 0) return "0";
  const lower = Math.floor(delta / 10) * 10;
  const sign = (n: number) => (n > 0 ? `+${n}` : `${n}`);
  return `${sign(lower)}..${sign(lower + 10)}`;
}
