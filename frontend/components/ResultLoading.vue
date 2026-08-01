<!-- Skeleton shown while the job is queued/running behind /today. -->
<script setup lang="ts">
import { computed } from "vue";
import { queuePhrase } from "~/config/queue-phrases";

const props = defineProps<{
  queuePosition?: number | null;
}>();

const message = computed(() =>
  props.queuePosition == null ? null : queuePhrase(props.queuePosition)
);
</script>

<template>
  <div class="result-loading" aria-busy="true" aria-label="Lädt Ergebnis">
    <div class="skeleton chip" />
    <div class="skeleton circle" />
    <div class="skeleton line short" />
    <div class="skeleton block" />
    <p v-if="queuePosition && queuePosition > 0" class="message">
      Warteschlangen-Position: {{ queuePosition }}
    </p>
    <p class="message">
      {{
        message ??
        "Wir schauen gerade live auf der Playerzone nach deinen Spielen. Das kann ein paar Sekunden dauern."
      }}
    </p>
  </div>
</template>

<style scoped>
.result-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 22px;
  padding: 0 20px 40px;
}

.skeleton {
  background: linear-gradient(
    100deg,
    color-mix(in srgb, var(--border-strong) 55%, var(--surface-2)) 30%,
    color-mix(in srgb, var(--text) 12%, var(--surface)) 50%,
    color-mix(in srgb, var(--border-strong) 55%, var(--surface-2)) 70%
  );
  background-size: 200% 100%;
  border-radius: var(--radius-sm);
}

@media not (prefers-reduced-motion: reduce) {
  .skeleton {
    animation: sheen 1.6s ease-in-out infinite;
  }
}

@keyframes sheen {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

.chip {
  width: 100%;
  height: 48px;
  border-radius: var(--radius-sm);
}

.circle {
  width: 208px;
  height: 208px;
  border-radius: 50%;
}

.line.short {
  width: 200px;
  height: 16px;
}

.block {
  width: 100%;
  height: 160px;
}

.message {
  margin: 0;
  font: 400 13px/1.5 var(--font-body);
  color: var(--text-muted);
  text-align: center;
  text-wrap: pretty;
  max-width: 280px;
}
</style>
