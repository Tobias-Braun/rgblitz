<!--
  Interstitial shown when the first fetch already returns cached/stale data
  (design 3h): let the user retry a live fetch or accept the stale snapshot.
-->
<script setup lang="ts">
defineProps<{
  retrying?: boolean
}>()

const emit = defineEmits<{
  retry: []
  'view-anyway': []
}>()
</script>

<template>
  <div class="result-stale">
    <div class="card">
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path
            d="M4 4.5V9.5H9"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
          <path
            d="M4.6 15A8 8 0 1 0 5.5 7.5L4 9.5"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
          <path d="M12 8.5V12.5L15 14.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </div>
      <p class="message">Die Daten konnten gerade nicht live geladen werden.</p>
      <p class="hint">Du siehst einen zwischengespeicherten Stand vom letzten erfolgreichen Abruf.</p>
    </div>
    <div class="actions">
      <button type="button" class="retry" :disabled="retrying" @click="emit('retry')">
        {{ retrying ? 'Versuche erneut…' : 'Erneut versuchen' }}
      </button>
      <button type="button" class="view-anyway" @click="emit('view-anyway')">Trotzdem anzeigen</button>
    </div>
  </div>
</template>

<style scoped>
.result-stale {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 24px;
  padding: 0 22px 40px;
}

.card {
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 32px 26px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 18px;
}

.icon {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  border: 1px solid var(--yellow);
  background: color-mix(in srgb, var(--yellow) 12%, transparent);
  color: var(--yellow);
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
  font: 400 16px/1.55 var(--font-body);
  color: rgba(244, 244, 242, 0.85);
  text-align: center;
  text-wrap: pretty;
}

.hint {
  margin: 0;
  font: 400 12px/1.5 var(--font-mono);
  color: var(--text-muted);
  text-align: center;
  text-wrap: pretty;
}

.actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
  max-width: 260px;
}

.retry {
  min-height: 44px;
  border-radius: var(--radius-sm);
  background: var(--green);
  color: #06231a;
  font: 600 15px/1 var(--font-body);
}

.retry:disabled {
  opacity: 0.6;
}

.view-anyway {
  min-height: 44px;
  font: 400 13px/1.4 var(--font-body);
  color: var(--text-muted);
}
</style>
