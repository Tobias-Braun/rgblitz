import { onUnmounted, ref, type Ref } from "vue";
import type { ApiError, JobResponse, TodayResult } from "~/types/api";

const POLL_INTERVAL_MS = 2000;

/**
 * Drives the async job-queue flow behind /api/players/:id/today:
 * `start`/`refresh` request a job (the backend returns a cached result
 * immediately if a fresh one exists), then this composable polls
 * /api/players/:id/today/jobs/:jobId every 2s while the job is
 * queued/running, exposing `queuePosition` for the loading UI, until the
 * job reaches a final status (done, error, or no_tournament).
 */
export function useTodayResult(playerId: Ref<string> | string) {
  const config = useRuntimeConfig();
  const data = ref<TodayResult | null>(null);
  const error = ref<string | null>(null);
  const pending = ref(false);
  const lastFetchedAt = ref<Date | null>(null);
  const jobFinishedAt = ref<Date | null>(null);
  const queuePosition = ref<number | null>(null);

  let pollTimer: ReturnType<typeof setInterval> | null = null;

  const resolveId = () =>
    typeof playerId === "string" ? playerId : playerId.value;

  function stopPolling() {
    if (pollTimer !== null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  function applyJobResponse(job: JobResponse) {
    if (job.status === "done" || job.status === "no_tournament") {
      stopPolling();
      pending.value = false;
      queuePosition.value = null;
      data.value = job.result ?? null;
      error.value = null;
      lastFetchedAt.value = new Date();
      jobFinishedAt.value = job.finished_at
        ? new Date(Date.parse(job.finished_at))
        : null;
      return;
    }

    if (job.status === "error") {
      stopPolling();
      pending.value = false;
      queuePosition.value = null;
      error.value = job.error ?? "upstream unavailable";
      return;
    }

    // queued | running — keep polling.
    queuePosition.value = job.queue_position ?? null;

    if (job.status === "queued" && job.job_id !== undefined) {
      startPolling(job.job_id);
    }
  }

  function startPolling(jobId: number) {
    stopPolling();
    pollTimer = setInterval(async () => {
      try {
        const job = await $fetch<JobResponse>(
          `/api/players/${resolveId()}/today/jobs/${jobId}`,
          {
            baseURL: config.public.apiBase,
          }
        );
        applyJobResponse(job);
      } catch (err) {
        stopPolling();
        pending.value = false;
        const apiError = err as { data?: ApiError };
        error.value = apiError.data?.error ?? "upstream unavailable";
      }
    }, POLL_INTERVAL_MS);
  }

  async function fetchOnce(): Promise<TodayResult | null> {
    stopPolling();
    pending.value = true;
    try {
      const job = await $fetch<JobResponse>(
        `/api/players/${resolveId()}/today`,
        {
          baseURL: config.public.apiBase,
        }
      );
      applyJobResponse(job);
      return data.value;
    } catch (err) {
      stopPolling();
      pending.value = false;
      const apiError = err as { data?: ApiError };
      error.value = apiError.data?.error ?? "upstream unavailable";
      return null;
    }
  }

  async function start(): Promise<TodayResult | null> {
    // Selecting a different player must not leave the previous player's
    // result (and its rgx delta) visible while the new fetch is in flight.
    stopPolling();
    data.value = null;
    error.value = null;
    lastFetchedAt.value = null;
    queuePosition.value = null;
    return fetchOnce();
  }

  onUnmounted(stopPolling);

  return {
    data,
    error,
    pending,
    lastFetchedAt,
    jobFinishedAt,
    queuePosition,
    start,
    refresh: fetchOnce,
  };
}
