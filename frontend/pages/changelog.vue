<script setup lang="ts"></script>

<template>
  <div class="page">
    <AppHeader />
    <main class="content">
      <NuxtLink to="/" class="back-link">← Zurück</NuxtLink>
      <h1 class="title">Changelog</h1>

      <section
        v-for="release in visibleReleases"
        :key="release.version"
        class="entry"
      >
        <h2 class="release">
          <span class="version">v{{ release.version }}</span>
          <span class="date">{{ formatDateRange(release.date, release.date) }}</span>
        </h2>
        <ul class="changes">
          <li v-for="change in release.changes" :key="change">{{ change }}</li>
        </ul>
      </section>
    </main>
  </div>
</template>

<style scoped>
.page {
  flex: 1;
  min-height: 0;
  width: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 22px;
  padding: 24px 18px 32px;
  padding-left: max(18px, calc((100% - var(--page-container-max-width)) / 2));
  padding-right: max(18px, calc((100% - var(--page-container-max-width)) / 2));
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

.back-link {
  align-self: flex-start;
  font: 500 13px/1 var(--font-body);
  color: var(--text-dim);
  text-decoration: none;
}

.back-link:hover {
  color: var(--text-muted);
}

.title {
  font: 700 26px/1 var(--font-display);
  color: var(--text);
  letter-spacing: -0.02em;
  margin: 0;
}

.entry {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 16px 18px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
}

.release {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
}

.version {
  font: 600 15px/1.3 var(--font-display);
  color: var(--text);
}

.date {
  font: 400 13px/1 var(--font-mono);
  color: var(--text-dim);
}

.changes {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding-left: 18px;
  font: 400 14px/1.55 var(--font-body);
  color: var(--text-muted);
}
</style>
