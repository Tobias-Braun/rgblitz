<!--
  Single route of the app (docs/04-architecture.md: "Single route / — all view
  transitions are CSS/JS state changes within the page, not router navigations").
-->
<script setup lang="ts">
import { ref, computed } from "vue";
import type { PlayerSummary, PlayerRef } from "~/types/api";
import { useTodayResult } from "~/composables/useTodayResult";

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

async function selectPlayer(player: PlayerSummary) {
  selectedPlayer.value = player;
  acknowledgedStale.value = false;
  await start();
}

function searchAgain() {
  selectedPlayer.value = null;
  acknowledgedStale.value = false;
  showFeedback.value = false;
}

async function retry() {
  acknowledgedStale.value = false;
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
      <PlayerChip :player="selectedPlayer" detailed @close="searchAgain" />
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
          @retry="retry"
          @search-again="searchAgain"
        />

        <NoTournamentCard
          v-else-if="data?.status === 'no_tournament'"
          @search-again="searchAgain"
        />

        <ResultStale
          v-else-if="data?.status === 'ok' && data.stale && !acknowledgedStale"
          :retrying="pending"
          @retry="retry"
          @view-anyway="viewAnyway"
        />

        <NoGamesPlayedYet
          v-else-if="data?.status === 'ok' && !data.games?.length"
          :player="data.player"
          :refreshing="pending"
          :last-fetched-at="lastFetchedAt"
          :job-finished-at="jobFinishedAt"
          @search-again="searchAgain"
          @refresh="refresh"
        />

        <ResultView
          v-else-if="data?.status === 'ok'"
          :key="data.player.id"
          :result="data"
          :refreshing="pending"
          :last-fetched-at="lastFetchedAt"
          :job-finished-at="jobFinishedAt"
          @search-again="searchAgain"
          @refresh="refresh"
          @feedback="showFeedback = true"
        />

        <ResultError
          v-else-if="fallbackPlayerRef"
          :retrying="pending"
          @retry="retry"
          @search-again="searchAgain"
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
