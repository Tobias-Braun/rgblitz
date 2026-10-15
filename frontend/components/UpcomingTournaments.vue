<!--
  Tournaments that haven't started yet, shown below the focused search bar
  until the user types. Styled like the autocomplete list it gives way to.
-->
<script setup lang="ts">
import type { UpcomingTournament } from "~/types/api";
import { formatDateRange } from "~/utils/format";

defineProps<{
  tournaments: UpcomingTournament[];
}>();
</script>

<template>
  <section>
    <h2 class="heading">Kommende Turniere</h2>
    <ul class="list">
      <li v-for="(t, i) in tournaments" :key="t.id" :style="{ '--i': i }">
        <a
          class="row"
          :href="`https://playerzone.roundnetgermany.de/tournaments/${t.id}`"
          target="_blank"
          rel="noopener"
        >
          <span class="name">{{ t.name }}</span>
          <span class="meta">
            {{ formatDateRange(t.start_date, t.end_date) }}
            <template v-if="t.location"> · {{ t.location }}</template>
          </span>
        </a>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.heading {
  margin: 0;
  padding: 8px 20px 4px;
  font: 400 11px/1.4 var(--font-mono);
  color: var(--text-muted);
}

.list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.list li {
  animation: fade-up 0.28s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  animation-delay: calc(var(--i, 0) * 35ms);
}

.row {
  min-height: 44px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 11px 20px;
  text-decoration: none;
}

.name {
  font: 500 15px/1.25 var(--font-body);
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.meta {
  font: 400 12px/1.3 var(--font-body);
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

@keyframes fade-up {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
