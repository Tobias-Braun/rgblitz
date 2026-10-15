<!--
  Single route of the app (docs/04-architecture.md: "Single route / — all view
  transitions are CSS/JS state changes within the page, not router navigations").
-->
<script setup lang="ts">
import { ref, computed, watch } from "vue";
import type { PlayerSummary, PlayerRef } from "~/types/api";
import { useTodayResult } from "~/composables/useTodayResult";
import { useTrack } from "~/composables/useTrack";
import { deltaBucket, resultState } from "~/utils/analytics";

const selectedPlayer = ref<PlayerSummary | null>(null);
const acknowledgedStale = ref(false);
const showFeedback = ref(false);

const playerId = computed(() => selectedPlayer.value?.id ?? "");
const {
  data,
  error,
  pending,
  lastFetchedAt,
  jobFinishedAt,
  queuePosition,
  start,
  refresh,
} = useTodayResult(playerId);

const { track } = useTrack();
let startedAt = 0;
let firstQueuePosition: number | undefined;

watch(queuePosition, (position) => {
  firstQueuePosition ??= position ?? undefined;
});

// One "result-shown" per settled job request (select, retry or refresh).
watch(pending, (isPending, wasPending) => {
  if (isPending) {
    startedAt = Date.now();
    firstQueuePosition = undefined;
    return;
  }
  if (!wasPending) return;

  const result = data.value;
  const state = resultState(result);
  track("result-shown", {
    state,
    games: result?.status === "ok" ? result.games?.length ?? 0 : 0,
    delta_bucket:
      result?.status === "ok" ? deltaBucket((result.total_delta ?? 0) / 2) : undefined,
    wait_s: Math.round((Date.now() - startedAt) / 1000),
    queue_pos: firstQueuePosition,
  });
  if (state === "error")
    track("job-error", { category: (error.value ?? "unknown").slice(0, 50) });
});

async function selectPlayer(player: PlayerSummary) {
  selectedPlayer.value = player;
  acknowledgedStale.value = false;
  await start();
}

function searchAgain(from: string) {
  track("search-again", { from });
  selectedPlayer.value = null;
  acknowledgedStale.value = false;
  showFeedback.value = false;
}

async function retry(from: string) {
  track("retry", { from });
  acknowledgedStale.value = false;
  await refresh();
}

async function onRefresh(from: string) {
  track("refresh", { from });
  await refresh();
}

function viewAnyway() {
  acknowledgedStale.value = true;
}

const fallbackPlayerRef = computed<PlayerRef | null>(() => {
  if (!selectedPlayer.value) return null;
  return {
    id: selectedPlayer.value.id,
    name: selectedPlayer.value.name,
    club_name: selectedPlayer.value.club_name,
    avatar_url: selectedPlayer.value.avatar_url,
    club_logo_url: selectedPlayer.value.club_logo_url,
    rgx: selectedPlayer.value.rgx,
  };
});
</script>

<template>
  <div class="page">
    <SearchBar v-if="!selectedPlayer" @select="selectPlayer" />

    <template v-else>
      <AppHeader />
      <PlayerChip
        :player="selectedPlayer"
        detailed
        @close="searchAgain('chip')"
      />
      <main class="content">
        <FeedbackForm v-if="showFeedback" @done="showFeedback = false" />

        <ResultLoading
          v-else-if="pending && !data"
          :queue-position="queuePosition"
        />

        <ResultError
          v-else-if="error && !data"
          :message="error"
          :retrying="pending"
          @retry="retry('error')"
          @search-again="searchAgain('error')"
        />

        <NoTournamentCard
          v-else-if="data?.status === 'no_tournament'"
          @search-again="searchAgain('no_tournament')"
        />

        <ResultStale
          v-else-if="data?.status === 'ok' && data.stale && !acknowledgedStale"
          :retrying="pending"
          @retry="retry('stale')"
          @view-anyway="viewAnyway"
        />

        <NoGamesPlayedYet
          v-else-if="data?.status === 'ok' && !data.games?.length"
          :player="data.player"
          :league="data.competition === 'league'"
          :refreshing="pending"
          :last-fetched-at="lastFetchedAt"
          :job-finished-at="jobFinishedAt"
          @search-again="searchAgain('no_games')"
          @refresh="onRefresh('no_games')"
        />

        <ResultView
          v-else-if="data?.status === 'ok'"
          :key="data.player.id"
          :result="data"
          :refreshing="pending"
          :last-fetched-at="lastFetchedAt"
          :job-finished-at="jobFinishedAt"
          @search-again="searchAgain('result')"
          @refresh="onRefresh('result')"
          @feedback="showFeedback = true"
        />

        <ResultError
          v-else-if="fallbackPlayerRef"
          :retrying="pending"
          @retry="retry('error')"
          @search-again="searchAgain('error')"
        />
      </main>
    </template>
  </div>
</template>

<style scoped>
.page {
  flex: 1;
  min-height: 0;
  width: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding-top: 24px;
  padding-left: max(0px, calc((100% - var(--page-container-max-width)) / 2));
  padding-right: max(0px, calc((100% - var(--page-container-max-width)) / 2));
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}
</style>
