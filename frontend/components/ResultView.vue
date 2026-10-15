<!--
  Orchestrates State 3 (design 3d/3e/3f): PlayerChip + RgxCircle + MoodText +
  SpieleListe, given today's tournament result.
-->
<script setup lang="ts">
import { computed, ref } from "vue";
import type { TodayResultOk } from "~/types/api";
import { useTrack } from "~/composables/useTrack";

const props = defineProps<{
  result: TodayResultOk;
  refreshing?: boolean;
  lastFetchedAt?: Date | null;
  jobFinishedAt?: Date | null;
}>();

const emit = defineEmits<{
  "search-again": [];
  refresh: [];
  feedback: [];
}>();

const before = computed(() => props.result.player.rgx);
// total_delta is the combined team delta; this is a per-player view, so
// halve it. The backend omits the field entirely (Go `omitempty`) when it's
// exactly 0, e.g. a day where games net out dead even.
const totalDeltaPerPlayer = computed(() => (props.result.total_delta ?? 0) / 2);
const totalPerformanceDeltaPerPlayer = computed(
  () => (props.result.total_performance_delta ?? 0) / 2
);
const after = computed(
  () => props.result.player.rgx + totalDeltaPerPlayer.value
);
const games = computed(() => props.result.games ?? []);

// Competition difficulty verdict based on gap between RGX and Performance
const competitionGap = computed(
  () => totalDeltaPerPlayer.value - totalPerformanceDeltaPerPlayer.value
);

const competitionVerdict = computed(() => {
  const gap = competitionGap.value;
  if (gap > 12) {
    return {
      category: "stark überbewertet",
      icon: "😌",
      color: "heavily-overvalued",
      explanation:
        "Du hast schlechter gespielt, als die Änderung deiner RGX suggeriert. Du hast gegen deutlich schwächere Gegner gespielt. Deine Performance ist weniger aussagekräftig als dein RGX-Ergebnis vermuten lässt.",
    };
  } else if (gap > 4) {
    return {
      category: "leicht überbewertet",
      icon: "🟡",
      color: "slightly-overvalued",
      explanation:
        "Du hast etwas schlechter gespielt, als die Änderung deiner RGX suggeriert. Du hast gegen schwächere Gegner gespielt als erwartet. Deine Performance ist etwas weniger beeindruckend als das Ergebnis zeigt.",
    };
  } else if (gap >= -4) {
    return {
      category: "normal",
      icon: "→",
      color: "normal",
      explanation:
        "Du hast genauso gespielt, wie die Änderung deiner RGX suggeriert. Du hast gegen Gegner der erwarteten Stärke gespielt. Deine Performance entspricht fair deinem heutigen RGX-Ergebnis.",
    };
  } else if (gap >= -12) {
    return {
      category: "leicht unterbewertet",
      icon: "💪",
      color: "slightly-undervalued",
      explanation:
        "Du hast besser gespielt, als die Änderung deiner RGX suggeriert. Du hast gegen stärkere Gegner gespielt als erwartet. Deine Performance ist beeindruckender als dein RGX-Ergebnis vermuten lässt.",
    };
  } else {
    return {
      category: "stark unterbewertet",
      icon: "🔥",
      color: "heavily-undervalued",
      explanation:
        "Du hast deutlich besser gespielt, als die Änderung deiner RGX suggeriert. Du hast gegen deutlich stärkere Gegner gespielt. Deine Performance war außergewöhnlich beeindruckend im Vergleich zu deinem RGX-Ergebnis.",
    };
  }
});

const isVerdictExpanded = ref(false);
const { track } = useTrack();

function toggleVerdict() {
  isVerdictExpanded.value = !isVerdictExpanded.value;
  if (isVerdictExpanded.value) track("verdict-expand");
}

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

    <div class="circle-container">
      <RgxCircle
        :key="result.player.id"
        :player-id="result.player.id"
        :before="before"
        :after="after"
        :total-delta="totalDeltaPerPlayer"
      />
    </div>
    <MoodText :total-delta="totalDeltaPerPlayer" />

    <p class="alpha-hint">
      Diese App befindet sich in der Alpha-Phase. Ergebnisse können fehlerhaft
      sein oder es kann zu Störungen kommen.
    </p>

    <SpieleListe :games="games" />

    <div class="verdict-accordion">
      <button
        type="button"
        class="verdict-trigger"
        :class="`verdict-${competitionVerdict.color}`"
        @click="toggleVerdict"
        :aria-expanded="isVerdictExpanded"
      >
        <span class="icon">{{ competitionVerdict.icon }}</span>
        <span class="verdict-label">Ergebnis-Interpretation</span>
        <span class="category">{{ competitionVerdict.category }}</span>
        <span class="chevron" :class="{ expanded: isVerdictExpanded }">›</span>
      </button>
      <div v-if="isVerdictExpanded" class="verdict-content">
        <p>{{ competitionVerdict.explanation }}</p>
      </div>
    </div>

    <p v-if="result.has_proxy" class="proxy-note">
      * Gegner-RGX geschätzt (neuer Spieler:in ohne Turnierhistorie)
    </p>

    <button
      type="button"
      class="feedback-cta"
      data-umami-event="feedback-open"
      @click="emit('feedback')"
    >
      Feedback geben
    </button>
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

.alpha-hint {
  margin: -6px 20px 0;
  font: 400 11px/1.5 var(--font-mono);
  color: var(--text-dim);
  text-align: center;
}

.proxy-note {
  margin: -6px 20px 0;
  font: 400 11px/1.5 var(--font-mono);
  color: var(--text-dim);
}

.circle-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.competition-verdict {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-radius: 8px;
  font: 500 12px/1 var(--font-mono);
  letter-spacing: 0.04em;
  margin-top: 8px;
}

.verdict-accordion {
  padding: 0 20px;
  width: 70%;
  margin: 0 auto;
}

@media (max-width: 768px) {
  .verdict-accordion {
    width: 100%;
  }
}

.verdict-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  border-radius: 8px;
  font: 500 12px/1 var(--font-mono);
  letter-spacing: 0.04em;
  margin-top: 8px;
  width: 100%;
  justify-content: flex-start;
  cursor: pointer;
  transition: background-color 0.2s ease;
}

.verdict-trigger:hover {
  background-color: color-mix(in srgb, currentColor 5%, transparent);
}

.verdict-label {
  opacity: 0.7;
}

.verdict-trigger .icon {
  font-size: 16px;
  flex: none;
}

.chevron {
  margin-left: auto;
  font-size: 20px;
  display: flex;
  align-items: center;
  transition: transform 0.2s ease;
  transform: rotate(90deg);
}

.chevron.expanded {
  transform: rotate(270deg);
}

.verdict-content {
  padding: 12px 16px;
  margin-top: 8px;
  font: 400 12px/1.5 var(--font-body);
  color: var(--text-dim);
  animation: expand 0.2s ease;
}

@keyframes expand {
  from {
    opacity: 0;
    max-height: 0;
  }
  to {
    opacity: 1;
    max-height: 200px;
  }
}

.verdict-heavily-overvalued {
  border: 1px solid var(--red);
  color: var(--red);
}

.verdict-slightly-overvalued {
  border: 1px solid var(--yellow);
  color: var(--yellow);
}

.verdict-normal {
  border: 1px solid var(--grey);
  color: var(--grey);
}

.verdict-slightly-undervalued {
  border: 1px solid var(--green);
  color: var(--green);
}

.verdict-heavily-undervalued {
  border: 1px solid var(--green);
  color: var(--green);
}

.feedback-cta {
  min-height: 44px;
  margin: 24px auto 0;
  padding: 0 24px;
  border-radius: var(--radius-sm);
  background: transparent;
  border: 1px solid var(--purple);
  color: var(--purple);
  font: 600 15px/1 var(--font-body);
}

@media (max-width: 768px) {
  .feedback-cta {
    margin-left: 20px;
    margin-right: 20px;
  }
}
</style>
