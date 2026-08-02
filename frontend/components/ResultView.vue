<!--
  Orchestrates State 3 (design 3d/3e/3f): PlayerChip + RgxCircle + MoodText +
  SpieleListe, given today's tournament result.
-->
<script setup lang="ts">
import { computed } from "vue";
import type { TodayResultOk } from "~/types/api";

const props = defineProps<{
  result: TodayResultOk;
  refreshing?: boolean;
  lastFetchedAt?: Date | null;
  jobFinishedAt?: Date | null;
}>();

const emit = defineEmits<{
  "search-again": [];
  refresh: [];
}>();

const before = computed(() => props.result.player.rgx);
// total_delta is the combined team delta; this is a per-player view, so
// halve it. The backend omits the field entirely (Go `omitempty`) when it's
// exactly 0, e.g. a day where games net out dead even.
const totalDeltaPerPlayer = computed(() => (props.result.total_delta ?? 0) / 2);
const after = computed(
  () => props.result.player.rgx + totalDeltaPerPlayer.value
);
const games = computed(() => props.result.games ?? []);

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
  <div class="result-view">
    <div class="status-row">
      <span class="fetched-at" :class="{ stale: result.stale }">
        {{ jobFinishedAtLabel }}
      </span>
      <button
        type="button"
        class="reload"
        :class="{ spinning: refreshing }"
        aria-label="Aktualisieren"
        @click="emit('refresh')"
      >
        ⟳
      </button>
    </div>

    <RgxCircle
      :key="result.player.id"
      :player-id="result.player.id"
      :before="before"
      :after="after"
      :total-delta="totalDeltaPerPlayer"
    />
    <MoodText :total-delta="totalDeltaPerPlayer" />

    <SpieleListe :games="games" />

    <p v-if="result.has_proxy" class="proxy-note">
      * Gegner-RGX geschätzt (neuer Spieler:in ohne Turnierhistorie)
    </p>
  </div>
</template>

<style scoped>
.result-view {
  display: flex;
  flex-direction: column;
  gap: 22px;
  padding-bottom: 40px;
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

.fetched-at.stale {
  color: var(--yellow);
}

.reload {
  display: grid;
  place-items: center;
  font-size: 24px;
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

.proxy-note {
  margin: -6px 20px 0;
  font: 400 11px/1.5 var(--font-mono);
  color: var(--text-dim);
}
</style>
