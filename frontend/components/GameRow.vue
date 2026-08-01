<!-- Single match row: opponent, W/L/D badge, signed delta (design 3e). -->
<script setup lang="ts">
import { computed } from "vue";
import type { Game } from "~/types/api";
import { deltaTone, formatDelta } from "~/utils/format";

const props = defineProps<{
  game: Game;
}>();

const rgxDeltaPerPlayer = computed(() =>
  props.game.rgx_delta ? props.game.rgx_delta / 2 : 0
);
const tone = computed(() => deltaTone(rgxDeltaPerPlayer.value));
</script>

<template>
  <div class="game-row">
    <span class="badge" :class="game.result.toLowerCase()">{{
      game.result
    }}</span>
    <div class="details">
      <span class="opponent">{{ game.opponent_name }}</span>
      <span v-if="game.format || game.sets?.length" class="meta">
        <span v-if="game.format">{{ game.format }}</span>
        <span v-if="game.sets?.length" class="chips">
          <span
            v-for="(set, index) in game.sets"
            :key="index"
            class="chip"
            :class="
              game.result === 'D'
                ? 'draw'
                : set === 'W'
                ? 'positive'
                : 'negative'
            "
            >{{ set }}</span
          >
        </span>
      </span>
    </div>
    <span class="delta" :class="tone">
      {{ formatDelta(rgxDeltaPerPlayer) }}
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
  width: 15px;
  height: 15px;
  border-radius: 4px;
  display: grid;
  place-items: center;
  font: 700 9px/1 var(--font-mono);
}

.chip.positive {
  background: color-mix(in srgb, var(--green) 20%, transparent);
  color: var(--green);
}

.chip.negative {
  background: color-mix(in srgb, var(--red) 20%, transparent);
  color: var(--red);
}

.chip.draw {
  background: color-mix(in srgb, var(--yellow) 20%, transparent);
  color: var(--yellow);
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
