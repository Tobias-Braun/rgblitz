<!-- State 2 — no tournament today for the selected player (design 3c). -->
<script setup lang="ts">
defineEmits<{
  "search-again": [];
}>();
</script>

<template>
  <div class="wrap">
    <div class="card">
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect
            x="3.5"
            y="5"
            width="17"
            height="15"
            rx="2.5"
            stroke="currentColor"
            stroke-width="1.5"
          />
          <path
            d="M3.5 9.5H20.5"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
          />
          <path
            d="M7.5 3V6.5"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
          />
          <path
            d="M16.5 3V6.5"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
          />
          <path
            d="M9 13.5L15 17.5"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
          />
          <path
            d="M15 13.5L9 17.5"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linecap="round"
          />
        </svg>
      </div>
      <p class="message">
        Es sieht so aus, als würdest du heute kein GRT Turnier spielen. Diese
        Seite funktioniert nur an Turnier-Tagen. Turniere werden erst
        registriert, wenn die Divisionen veröffentlicht worden sind.
      </p>
      <p class="hint">Turniere sind nur Sa &amp; So.</p>
    </div>
    <button type="button" class="search-again" @click="$emit('search-again')">
      Andere:n Spieler:in suchen
    </button>
  </div>
</template>

<style scoped>
.wrap {
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
  font: 400 16px/1.55 var(--font-body);
  color: rgba(244, 244, 242, 0.85);
  text-align: center;
  text-wrap: pretty;
}

.hint {
  margin: 0;
  font: 400 12px/1.4 var(--font-mono);
  color: var(--text-muted);
  text-align: center;
}

.search-again {
  min-height: 44px;
  font: 400 12px/1.4 var(--font-body);
  color: var(--text-muted);
}
</style>
