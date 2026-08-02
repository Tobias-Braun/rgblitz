<!-- Single match row: opponent, W/L/D badge, signed delta (design 3e). -->
<script setup lang="ts">
import { computed } from "vue";
import type { Game } from "~/types/api";
import { deltaTone, formatDelta } from "~/utils/format";

const props = defineProps<{
  game: Game;
}>();

const tone = computed(() =>
  props.game.was_cancelled
    ? "neutral"
    : deltaTone(props.game.rgx_delta_per_player)
);
</script>

<template>
  <div class="game-row" :class="{ cancelled: game.was_cancelled }">
    <span class="badge" :class="game.result.toLowerCase()">{{
      game.result
    }}</span>
    <div class="details">
      <span class="opponent">{{ game.opponent_name }}</span>
      <span v-if="game.was_cancelled || game.sets.length" class="meta">
        <span v-if="game.was_cancelled">Abgesagt</span>
        <span v-if="game.sets.length" class="chips">
          <span v-for="(set, index) in game.sets" :key="index" class="chip">
            {{ set.our_score }}:{{ set.opponent_score }}
          </span>
        </span>
      </span>
    </div>
    <span class="delta" :class="tone">
      {{ game.was_cancelled ? "—" : formatDelta(game.rgx_delta_per_player) }}
      <sup
        v-if="game.has_proxy"
        class="proxy-marker"
        title="Gegner-RGX geschätzt"
        >*</sup
      >
    </span>
  </div>
</template>

<style scoped>
.game-row {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 44px;
  padding: 11px 20px;
  border-top: 1px solid var(--border);
}

.badge {
  width: 24px;
  height: 24px;
  border-radius: 7px;
  flex: none;
  display: grid;
  place-items: center;
  font: 700 11px/1 var(--font-display);
}

.badge.w {
  background: color-mix(in srgb, var(--green) 18%, transparent);
  color: var(--green);
}

.badge.l {
  background: color-mix(in srgb, var(--red) 18%, transparent);
  color: var(--red);
}

.badge.d {
  background: color-mix(in srgb, var(--yellow) 18%, transparent);
  color: var(--yellow);
}

.badge.c {
  background: color-mix(in srgb, var(--grey) 18%, transparent);
  color: var(--grey);
}

.game-row.cancelled {
  opacity: 0.6;
}

.details {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.opponent {
  font: 500 14px/1.3 var(--font-body);
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font: 400 11px/1 var(--font-mono);
  color: var(--text-dim);
}

.chips {
  display: flex;
  gap: 4px;
}

.chip {
  padding: 1px 4px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--grey) 12%, transparent);
  font: 700 11px/1.4 var(--font-mono);
}

.delta {
  flex: none;
  font: 600 14px/1 var(--font-mono);
  font-variant-numeric: tabular-nums;
}

.delta.positive {
  color: var(--green);
}

.delta.negative {
  color: var(--red);
}

.delta.neutral {
  color: var(--grey);
}

.proxy-marker {
  color: var(--text-dim);
}
</style>
