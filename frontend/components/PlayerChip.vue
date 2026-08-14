<!-- Collapsed player identity row shown above the result/no-tournament states (design 3c/3d/3e/3f). -->
<script setup lang="ts">
import { ref } from "vue";
import type { PlayerRef } from "~/types/api";
import { initials, withApiBase } from "~/utils/format";

const props = withDefaults(
  defineProps<{
    player: PlayerRef;
    detailed?: boolean;
  }>(),
  { detailed: false }
);

defineEmits<{
  close: [];
}>();

const config = useRuntimeConfig();
const avatarFailed = ref(false);
const clubLogoFailed = ref(false);
</script>

<template>
  <div class="player-chip">
    <div class="avatar">
      <img
        v-if="player.avatar_url && !avatarFailed"
        :src="withApiBase(player.avatar_url, config.public.apiBase)"
        width="26"
        height="26"
        loading="lazy"
        alt=""
        @error="avatarFailed = true"
      />
      <span v-else class="avatar-fallback">{{
        initials(props.player.name)
      }}</span>
    </div>
    <div class="identity">
      <div class="name">{{ player.name }}</div>
      <div v-if="detailed" class="subtitle">RGX {{ player.rgx }}</div>
    </div>
    <div v-if="player.club_logo_url && !clubLogoFailed" class="club-logo">
      <img
        :src="withApiBase(player.club_logo_url, config.public.apiBase)"
        width="22"
        height="22"
        loading="lazy"
        alt=""
        @error="clubLogoFailed = true"
      />
    </div>
    <button
      type="button"
      class="close"
      aria-label="Andere:n Spieler:in suchen"
      @click="$emit('close')"
    >
      ×
    </button>
  </div>
</template>

<style scoped>
.player-chip {
  height: 48px;
  border-radius: var(--radius-sm);
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid var(--border-strong);
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 0 14px;
  margin: 12px max(18px, calc((100% - var(--page-container-max-width)) / 2))
    14px;
}

.avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  flex: none;
  overflow: hidden;
  display: grid;
  place-items: center;
  background: repeating-linear-gradient(135deg, #22252b 0 4px, #191c21 4px 8px);
}

.avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-fallback {
  font: 600 9px/1 var(--font-display);
  color: var(--text-muted);
}

.identity {
  flex: 1;
  min-width: 0;
}

.name {
  font: 500 15px/1.2 var(--font-body);
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.subtitle {
  font: 400 11px/1.3 var(--font-mono);
  color: var(--text-muted);
  margin-top: 2px;
}

.club-logo {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  flex: none;
  overflow: hidden;
  display: grid;
  place-items: center;
}

.club-logo img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.close {
  width: 44px;
  height: 44px;
  margin-right: -10px;
  display: grid;
  place-items: center;
  font: 400 20px/1 var(--font-body);
  color: var(--text-muted);
  flex: none;
}
</style>
