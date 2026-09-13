<!--
  Gradient glow ring + cumulative daily delta (design 3d/3e/3f). Glow color
  and pulse animation depend on sign and battery/motion state.
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useBattery } from "~/composables/useBattery";
import { deltaTone, formatDelta } from "~/utils/format";

const props = defineProps<{
  before: number;
  after: number;
  totalDelta: number;
  totalPerformanceDelta?: number;
  playerId: string;
}>();

const { isFastMode } = useBattery();
const tone = computed(() => deltaTone(props.totalDelta));

const COUNT_UP_DURATION_MS = 1200;
// Caps visible ticks so the counter reads as a smooth-ish climb rather than
// a flicker of every animation frame, and rounds to whole numbers while
// counting so the digit count doesn't jitter (e.g. "12" -> "12.5" -> "13").
const COUNT_UP_STEPS = 18;

// Counts up in absolute value (0 -> |target|, sign fixed for the whole
// animation) regardless of whether the delta is positive or negative.
const displayDelta = ref(0);
let rafId: number | null = null;

function animateCountUp(target: number) {
  if (rafId !== null) cancelAnimationFrame(rafId);

  if (isFastMode.value) {
    displayDelta.value = target;
    return;
  }

  const sign = target < 0 ? -1 : 1;
  const targetAbs = Math.abs(target);
  const startTime = performance.now();
  let lastStep = -1;

  const tick = (now: number) => {
    const progress = Math.min((now - startTime) / COUNT_UP_DURATION_MS, 1);
    const step = Math.floor(progress * COUNT_UP_STEPS);

    if (step !== lastStep || progress === 1) {
      lastStep = step;
      if (progress < 1) {
        const eased = 1 - Math.pow(1 - progress, 3);
        displayDelta.value = Math.round(sign * targetAbs * eased);
      } else {
        displayDelta.value = target;
      }
    }

    if (progress < 1) {
      rafId = requestAnimationFrame(tick);
    } else {
      rafId = null;
    }
  };
  rafId = requestAnimationFrame(tick);
}

onMounted(() => animateCountUp(props.totalDelta));
watch(
  () => props.totalDelta,
  (value) => animateCountUp(value)
);
onBeforeUnmount(() => {
  if (rafId !== null) cancelAnimationFrame(rafId);
});
</script>

<template>
  <div class="rgx-circle" :class="[tone, { static: isFastMode }]">
    <div class="ring">
      <div v-if="!isFastMode" class="glow" />
      <div class="core">
        <span class="delta" :class="{ static: isFastMode }">{{
          formatDelta(displayDelta)
        }}</span>
        <span class="before-after">{{ before }} → {{ after }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rgx-circle {
  --tone-color: var(--grey);
  display: grid;
  place-items: center;
}

.rgx-circle.positive {
  --tone-color: var(--green);
}

.rgx-circle.negative {
  --tone-color: var(--red);
}

.ring {
  position: relative;
  width: 208px;
  height: 208px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  background: radial-gradient(circle, var(--surface) 60%, transparent 61%);
  border: 3px solid var(--tone-color);
  box-shadow: 0 0 0 1px var(--border) inset;
}

.rgx-circle:not(.static) .ring {
  animation: pulse 2.4s ease-in-out infinite;
}

.glow {
  position: absolute;
  inset: -18px;
  border-radius: 50%;
  background: radial-gradient(
    circle,
    color-mix(in srgb, var(--tone-color) 35%, transparent),
    transparent 70%
  );
  filter: blur(6px);
  z-index: -1;
}

@keyframes pulse {
  0%,
  100% {
    box-shadow: 0 0 0 1px var(--border) inset,
      0 0 0 0 color-mix(in srgb, var(--tone-color) 45%, transparent);
  }
  50% {
    box-shadow: 0 0 0 1px var(--border) inset, 0 0 0 14px transparent;
  }
}

.core {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.delta {
  font: 800 68px/1 var(--font-display);
  color: var(--tone-color);
  letter-spacing: -0.03em;
  animation: delta-in 0.5s cubic-bezier(0.2, 0.8, 0.2, 1) both;
}

.delta.static {
  animation: none;
}

.before-after {
  font: 500 13px/1 var(--font-mono);
  color: var(--text-dim);
}

@keyframes delta-in {
  0% {
    opacity: 0;
    transform: scale(0.7);
  }
  60% {
    opacity: 1;
    transform: scale(1.08);
  }
  100% {
    opacity: 1;
    transform: scale(1);
  }
}
</style>
