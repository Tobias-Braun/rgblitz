<!--
  State 1 (docs/02-ui-requirements.md): idle hero + pill search bar (design
  3a), focused full-width bar + autocomplete (design 3b), empty-result
  message (design 3g).
-->
<script setup lang="ts">
import { ref, watch, nextTick } from "vue";
import type { PlayerSummary } from "~/types/api";
import { useBattery } from "~/composables/useBattery";

const emit = defineEmits<{
  select: [player: PlayerSummary];
}>();

const { isFastMode } = useBattery();
const config = useRuntimeConfig();

const inputEl = ref<HTMLInputElement | null>(null);
const query = ref("");
const isFocused = ref(false);
const results = ref<PlayerSummary[]>([]);
const pending = ref(false);
const searched = ref(false);

let debounceTimer: ReturnType<typeof setTimeout> | null = null;

watch(query, (value) => {
  if (debounceTimer) clearTimeout(debounceTimer);

  const trimmed = value.trim();
  if (trimmed.length < 2) {
    results.value = [];
    searched.value = false;
    return;
  }

  debounceTimer = setTimeout(async () => {
    pending.value = true;
    try {
      results.value = await $fetch<PlayerSummary[]>("/api/players/search", {
        baseURL: config.public.apiBase,
        params: { q: trimmed },
      });
    } catch {
      results.value = [];
    } finally {
      searched.value = true;
      pending.value = false;
    }
  }, 200);
});

function onFocus() {
  isFocused.value = true;
}

function cancel() {
  query.value = "";
  results.value = [];
  searched.value = false;
  isFocused.value = false;
  inputEl.value?.blur();
}

function selectPlayer(player: PlayerSummary) {
  emit("select", player);
}

async function focusInput() {
  isFocused.value = true;
  await nextTick();
  inputEl.value?.focus();
}
</script>

<template>
  <div class="search-view">
    <div class="backdrop" :class="{ 'fast-mode': isFastMode }">
      <template v-if="!isFastMode && !isFocused">
        <div class="triangle-wrap" :class="{ dimmed: isFocused }">
          <div class="triangle">
            <div class="blob blob-a" />
            <div class="blob blob-b" />
            <div class="blob blob-c" />
          </div>
        </div>
        <div class="vignette" />
      </template>
    </div>

    <div class="header-bar">
      <AppHeader />
    </div>

    <div v-if="isFocused" class="top-bar">
      <label class="bar bar--focused">
        <svg
          class="magnifier magnifier--active"
          viewBox="0 0 20 20"
          fill="none"
          aria-hidden="true"
        >
          <circle
            cx="8.5"
            cy="8.5"
            r="6"
            stroke="currentColor"
            stroke-width="1.6"
          />
          <line
            x1="13.1"
            y1="13.1"
            x2="17.5"
            y2="17.5"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
          />
        </svg>
        <input
          ref="inputEl"
          v-model="query"
          type="text"
          placeholder="Spieler:in suchen…"
          autocomplete="off"
        />
      </label>
      <button type="button" class="cancel" @click="cancel">Abbr.</button>
    </div>

    <div v-else class="hero">
      <div class="hero-copy">
        <h1>Dein RGX,<br />noch auf dem Feld.</h1>
        <p>
          Such deinen Namen und sieh sofort, wie viele Punkte du heute geholt
          oder verloren hast. Spiel für Spiel, live vom Turnier.
        </p>
      </div>
      <button type="button" class="bar bar--idle" @click="focusInput">
        <svg
          class="magnifier"
          viewBox="0 0 20 20"
          fill="none"
          aria-hidden="true"
        >
          <circle
            cx="8.5"
            cy="8.5"
            r="6"
            stroke="currentColor"
            stroke-width="1.6"
          />
          <line
            x1="13.1"
            y1="13.1"
            x2="17.5"
            y2="17.5"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
          />
        </svg>
        <span class="placeholder">Spieler:in suchen…</span>
      </button>
      <p class="hint">nur an Turniertagen · Sa &amp; So</p>
    </div>

    <div v-if="isFocused" class="results">
      <template
        v-if="
          query.trim().length >= 2 &&
          searched &&
          results.length === 0 &&
          !pending
        "
      >
        <div class="empty">
          <p class="empty-title">Kein:e Spieler:in gefunden</p>
          <p class="empty-sub">
            Der Index wird samstags und sonntags um 03:00 aktualisiert.
          </p>
        </div>
      </template>
      <template v-else-if="results.length > 0">
        <ul class="list">
          <li
            v-for="(player, i) in results"
            :key="player.id"
            :style="{ '--i': i }"
          >
            <AutocompleteItem :player="player" @select="selectPlayer" />
          </li>
        </ul>
        <p class="footnote">max. 10 Treffer · nach RGX sortiert</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.search-view {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.backdrop {
  position: absolute;
  inset: 0;
  z-index: 0;
  overflow: hidden;
}

.backdrop.fast-mode {
  background: linear-gradient(180deg, #12161a, #0c0d10);
}

/*
 * The three glows sit at the vertices of an equilateral triangle inscribed
 * in `.triangle` (480x480, center at 240/240, vertex radius 130px). `.triangle`
 * itself slowly rotates, carrying all three vertices around together; each
 * `.blob` independently pulses (scale + opacity) on its own cycle so the
 * group doesn't look like it's breathing in lockstep.
 */
.triangle-wrap {
  position: absolute;
  left: 50%;
  top: 46%;
  width: 480px;
  height: 480px;
  transform: translate(-50%, -50%);
  transition: opacity 0.3s ease;
}

.triangle-wrap.dimmed {
  opacity: 0.35;
}

.triangle {
  position: absolute;
  inset: 0;
  animation: rotate-triangle 25s linear infinite;
}

.blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(50px);
  will-change: transform;
}

.blob-a {
  width: 470px;
  height: 470px;
  left: 240px;
  top: 110px;
  background: radial-gradient(
    circle,
    rgba(63, 224, 174, 0.45),
    transparent 65%
  );
  animation: pulse-glow 10s ease-in-out infinite;
}

.blob-b {
  width: 430px;
  height: 430px;
  left: 353px;
  top: 305px;
  background: radial-gradient(
    circle,
    rgba(168, 92, 255, 0.42),
    transparent 65%
  );
  animation: pulse-glow 10s ease-in-out infinite -3.5s;
}

.blob-c {
  width: 430px;
  height: 430px;
  left: 127px;
  top: 305px;
  background: radial-gradient(
    circle,
    rgba(255, 149, 66, 0.42),
    transparent 65%
  );
  animation: pulse-glow 10s ease-in-out infinite -7s;
}

.vignette {
  position: absolute;
  inset: 0;
  background: radial-gradient(
    ellipse at 50% 46%,
    transparent 16%,
    rgba(11, 12, 15, 0.3) 76%
  );
}

@keyframes rotate-triangle {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

@keyframes pulse-glow {
  0%,
  100% {
    transform: translate(-50%, -50%) scale(0.7);
    opacity: 0.75;
  }
  50% {
    transform: translate(-50%, -50%) scale(1);
    opacity: 1;
  }
}

.header-bar {
  position: relative;
  z-index: 2;
}

.hero {
  position: relative;
  z-index: 1;
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 34px;
  padding: 0 30px 70px;
}

.hero-copy {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}

.hero-copy h1 {
  margin: 0;
  font: 700 34px/1.15 var(--font-display);
  color: var(--text);
  text-align: center;
  letter-spacing: -0.02em;
  text-wrap: pretty;
  animation: fade-up 0.6s cubic-bezier(0.2, 0.8, 0.2, 1) both;
}

.hero-copy p {
  margin: 0;
  font: 400 16px/1.55 var(--font-body);
  color: var(--text-muted);
  text-align: center;
  text-wrap: pretty;
  max-width: 290px;
  animation: fade-up 0.6s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  animation-delay: 0.08s;
}

.hint {
  margin: 0;
  font: 400 12px/1.5 var(--font-mono);
  color: var(--text-dim);
  text-align: center;
  animation: fade-up 0.6s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  animation-delay: 0.24s;
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

.bar {
  display: flex;
  align-items: center;
  gap: 11px;
}

.bar--idle {
  width: min(268px, 100%);
  height: 54px;
  border-radius: 27px;
  background: rgba(255, 255, 255, 0.07);
  border: 1px solid var(--border-strong);
  backdrop-filter: blur(12px);
  padding: 0 20px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.45);
  animation: fade-up 0.6s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  animation-delay: 0.16s;
}

.placeholder {
  font: 400 16px/1 var(--font-body);
  color: var(--text-muted);
}

.top-bar {
  position: relative;
  z-index: 1;
  padding: 10px 16px 14px;
  display: flex;
  align-items: center;
  gap: 10px;
  background: rgba(13, 14, 18, 0.92);
  backdrop-filter: blur(16px);
  border-bottom: 1px solid var(--border);
}

.bar--focused {
  flex: 1;
  height: 48px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid var(--border-strong);
  box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.08);
  padding: 0 16px;
}

.bar--focused input {
  flex: 1;
  min-width: 0;
  height: 100%;
  font: 400 16px/1 var(--font-body);
  color: var(--text);
  outline: none;
}

.bar--focused input::placeholder {
  color: var(--text-muted);
}

.cancel {
  min-height: 44px;
  font: 500 14px/1 var(--font-body);
  color: var(--text-muted);
  padding: 0 4px;
}

.magnifier {
  width: 17px;
  height: 17px;
  flex: none;
  color: rgba(244, 244, 242, 0.6);
}

.magnifier--active {
  color: var(--text);
}

.results {
  position: relative;
  z-index: 1;
  flex: 1;
  overflow-y: auto;
  padding: 6px 0;
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

.footnote {
  margin: 0;
  padding: 14px 20px 0;
  font: 400 11px/1.4 var(--font-mono);
  color: var(--text-muted);
}

.empty {
  padding: 36px 32px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}

.empty-title {
  margin: 0;
  font: 500 15px/1.4 var(--font-body);
  color: rgba(244, 244, 242, 0.85);
}

.empty-sub {
  margin: 0;
  font: 400 13px/1.5 var(--font-body);
  color: var(--text-muted);
  text-align: center;
  text-wrap: pretty;
}
</style>
