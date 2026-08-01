<!-- Shown when the backend is unreachable and no cached data exists (503). -->
<script setup lang="ts">
defineProps<{
  message?: string
  retrying?: boolean
}>()

const emit = defineEmits<{
  retry: []
  'search-again': []
}>()
</script>

<template>
  <div class="result-error">
    <div class="card">
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path
            d="M12 3.5L21.5 20H2.5L12 3.5Z"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linejoin="round"
          />
          <path d="M12 10V14.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
          <circle cx="12" cy="17.3" r="1" fill="currentColor" />
        </svg>
      </div>
      <p class="message">Playerzone ist gerade nicht erreichbar.</p>
      <p v-if="message" class="detail">{{ message }}</p>
    </div>
    <div class="actions">
      <button type="button" class="retry" :disabled="retrying" @click="emit('retry')">
        {{ retrying ? 'Versuche erneut…' : 'Erneut versuchen' }}
      </button>
      <button type="button" class="search-again" @click="emit('search-again')">
        Andere:n Spieler:in suchen
      </button>
    </div>
  </div>
</template>

<style scoped>
.result-error {
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
  gap: 14px;
}

.icon {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  border: 1px solid var(--red);
  background: color-mix(in srgb, var(--red) 12%, transparent);
  color: var(--red);
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

.detail {
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
  background: var(--red);
  color: #2a0a03;
  font: 600 15px/1 var(--font-body);
}

.retry:disabled {
  opacity: 0.6;
}

.search-again {
  min-height: 44px;
  font: 400 13px/1.4 var(--font-body);
  color: var(--text-muted);
}
</style>
