<!--
  Player is in an active tournament (or their league team plays a matchday) today
  but has no games yet (backend omits `games`/`total_delta` entirely — see
  scraper.ScrapePlayerToday and scraper.ScrapePlayerLeagueToday). Distinct from State 2 (no tournament at all):
  here we keep the player chip + a manual refresh, since games can appear at
  any moment.
-->
<script setup lang="ts">
import { computed } from "vue";
import type { PlayerRef } from "~/types/api";

const props = defineProps<{
  player: PlayerRef;
  league?: boolean;
  refreshing?: boolean;
  lastFetchedAt?: Date | null;
  jobFinishedAt?: Date | null;
}>();

defineEmits<{
  "search-again": [];
  refresh: [];
}>();

const timeFormatter = new Intl.DateTimeFormat("de-DE", {
  hour: "2-digit",
  minute: "2-digit",
});

const jobFinishedAtLabel = computed(() => {
  if (!props.jobFinishedAt) return "";
  return `Stand ${timeFormatter.format(props.jobFinishedAt)} Uhr`;
});
</script>

<template>
  <div class="no-games">
    <PlayerChip :player="player" detailed @close="$emit('search-again')" />

    <div class="status-row">
      <span class="fetched-at">{{ jobFinishedAtLabel }}</span>
      <button
        type="button"
        class="reload"
        :class="{ spinning: refreshing }"
        aria-label="Aktualisieren"
        @click="$emit('refresh')"
      >
        ⟳
      </button>
    </div>

    <div class="card">
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <circle
            cx="12"
            cy="12"
            r="8.5"
            stroke="currentColor"
            stroke-width="1.5"
          />
          <path
            d="M12 7.5V12L15 14"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </div>
      <p class="message">Noch keine Spiele heute</p>
      <p v-if="league" class="hint">
        Dein Team hat heute einen Liga-Spieltag, aber es wurde noch keine
        Partie mit dir erfasst. Schau später noch mal rein.
      </p>
      <p v-else class="hint">
        Du bist für ein Turnier eingetragen, aber es wurden noch keine
        Ergebnisse erfasst. Schau später noch mal rein.
      </p>
    </div>
  </div>
</template>

<style scoped>
.no-games {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}

.fetched-at {
  font: 500 12px/1 var(--font-mono);
  color: var(--text-muted);
  letter-spacing: 0.04em;
}

.reload {
  width: 44px;
  height: 44px;
  display: grid;
  place-items: center;
  font-size: 17px;
  color: var(--text-muted);
}

.reload.spinning {
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.card {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 18px;
  padding: 32px 26px 60px;
}

.icon {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  border: 1px solid var(--border-strong);
  background: color-mix(in srgb, var(--grey) 12%, transparent);
  color: var(--grey);
  display: flex;
  align-items: center;
  justify-content: center;
}

.icon svg {
  width: 26px;
  height: 26px;
}

.message {
  margin: 0;
  font: 600 17px/1.4 var(--font-display);
  color: var(--text);
  text-align: center;
  text-wrap: pretty;
}

.hint {
  margin: 0;
  font: 400 13px/1.55 var(--font-body);
  color: var(--text-muted);
  text-align: center;
  text-wrap: pretty;
  max-width: 280px;
}
</style>
