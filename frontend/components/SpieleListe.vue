<!-- "Spiele" accordion, collapsed by default (design 3d collapsed / 3e expanded). -->
<script setup lang="ts">
import { ref } from 'vue'
import type { Game } from '~/types/api'

const props = defineProps<{
  games: Game[]
}>()

const isOpen = ref(false)

function toggle() {
  isOpen.value = !isOpen.value
}
</script>

<template>
  <div class="spiele-liste">
    <button type="button" class="header" :aria-expanded="isOpen" @click="toggle">
      <span>Spiele heute</span>
      <span class="count">{{ props.games.length }}</span>
      <svg
        class="chevron"
        :class="{ open: isOpen }"
        width="16"
        height="16"
        viewBox="0 0 16 16"
        fill="none"
      >
        <path
          d="M4 6l4 4 4-4"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        />
      </svg>
    </button>
    <div class="panel" :class="{ open: isOpen }">
      <div class="panel-inner">
        <GameRow v-for="(game, index) in props.games" :key="index" :game="game" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.spiele-liste {
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
}

.header {
  width: 100%;
  min-height: 44px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 13px 20px;
  font: 500 15px/1 var(--font-body);
  color: var(--text);
}

.count {
  font: 500 12px/1 var(--font-mono);
  color: var(--text-dim);
  background: var(--surface-2);
  border-radius: 999px;
  padding: 3px 8px;
}

.chevron {
  flex: none;
  margin-left: auto;
  color: var(--text-muted);
  transform-origin: 50% 50%;
  transition: transform 0.2s ease;
}

.chevron.open {
  transform: rotate(180deg);
}

.panel {
  display: grid;
  grid-template-rows: 0fr;
  transition: grid-template-rows 0.25s ease;
}

.panel.open {
  grid-template-rows: 1fr;
}

.panel-inner {
  overflow: hidden;
}

@media (prefers-reduced-motion: reduce) {
  .panel {
    transition: none;
  }
}
</style>
