import type { RgxJobStats } from "~/types/api";

export function useRgxJobStats() {
  const config = useRuntimeConfig();
  return useFetch<RgxJobStats>("/api/stats/rgx-jobs", {
    baseURL: config.public.apiBase,
    key: "rgx-job-stats",
    lazy: true,
    server: true,
  });
}
