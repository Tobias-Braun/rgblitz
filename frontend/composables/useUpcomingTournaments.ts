import type { UpcomingTournament } from "~/types/api";

export function useUpcomingTournaments() {
  const config = useRuntimeConfig();
  return useFetch<UpcomingTournament[]>("/api/tournaments/upcoming", {
    baseURL: config.public.apiBase,
    key: "upcoming-tournaments",
    lazy: true,
    server: true,
  });
}
