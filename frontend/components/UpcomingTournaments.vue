<!--
  Start page list of tournaments that haven't started yet. Rendered only once
  entries are known — loading, errors and an empty list leave no trace.
-->
<script setup lang="ts">
import { useUpcomingTournaments } from "~/composables/useUpcomingTournaments";
import { formatDateRange } from "~/utils/format";

const { data: tournaments } = useUpcomingTournaments();
</script>

<template>
  <section v-if="tournaments?.length" class="upcoming">
    <h2 class="heading">Kommende Turniere</h2>
    <ul class="list">
      <li v-for="t in tournaments" :key="t.id">
        <a
          class="row"
          :href="`https://playerzone.roundnetgermany.de/tournaments/${t.id}`"
          target="_blank"
          rel="noopener"
        >
          <span class="name">{{ t.name }}</span>
          <span class="meta">
            <span class="date">{{ formatDateRange(t.start_date, t.end_date) }}</span>
            <span v-if="t.location">· {{ t.location }}</span>
          </span>
        </a>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.upcoming {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: var(--page-container-max-width);
  margin: 0 auto;
  padding-bottom: 24px;
}

.heading {
  margin: 0;
  padding: 0 18px 12px;
  font: 400 12px/1.5 var(--font-mono);
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.list {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--border);
}

.list li {
  border-bottom: 1px solid var(--border);
}

.row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 18px;
  color: inherit;
  text-decoration: none;
}

.row:hover,
.row:focus-visible {
  background: rgba(255, 255, 255, 0.04);
}

.name {
  font: 600 16px/1.3 var(--font-display);
  color: var(--text);
}

.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  font: 400 14px/1.4 var(--font-body);
  color: var(--text-muted);
}

.date {
  font-family: var(--font-mono);
  font-size: 13px;
}
</style>
