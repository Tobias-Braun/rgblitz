<!-- Logo/wordmark bar shown on every screen except the focused search input (design 2c). -->
<script setup lang="ts"></script>

<template>
  <header class="app-header">
    <div class="logo-tile">
      <div class="bolt" />
    </div>
    <div class="wordmark">
      <span class="rg">RG</span>
      <span class="blitz">BLITZ</span>
    </div>
    <span class="alpha">ALPHA</span>
  </header>
</template>

<style scoped>
.app-header {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 10px max(18px, calc((100% - var(--page-container-max-width)) / 2));
  flex: none;
  border-bottom: 1px solid var(--border);
}

.logo-tile {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  background: linear-gradient(160deg, var(--amber-1), var(--amber-2));
  display: grid;
  place-items: center;
  flex: none;
}

.bolt {
  width: 10px;
  height: 16px;
  background: var(--bg);
  clip-path: polygon(
    46% 0,
    100% 0,
    54% 42%,
    94% 42%,
    16% 100%,
    48% 54%,
    2% 54%
  );
}

.wordmark {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.rg {
  font: 700 18px/1 var(--font-display);
  color: var(--text);
  letter-spacing: -0.03em;
}

.blitz {
  font: 500 9px/1 var(--font-mono);
  letter-spacing: 0.28em;
  color: var(--amber-text);
}

.alpha {
  margin-left: auto;
  align-self: center;
  font: 600 12px/1 var(--font-mono);
  letter-spacing: 0.16em;
  color: var(--text-dim);
  border: 1px solid var(--border);
  border-radius: 5px;
  padding: 3px 6px;
}
</style>
