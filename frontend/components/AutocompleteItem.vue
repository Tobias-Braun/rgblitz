<!-- Single autocomplete row: avatar, name, club logo, RGX (design 3b). -->
<script setup lang="ts">
import { ref } from 'vue'
import type { PlayerSummary } from '~/types/api'
import { initials, withApiBase } from '~/utils/format'

const props = defineProps<{
  player: PlayerSummary
}>()

defineEmits<{
  select: [player: PlayerSummary]
}>()

const config = useRuntimeConfig()
const avatarFailed = ref(false)
const clubLogoFailed = ref(false)
</script>

<template>
  <button type="button" class="row" @click="$emit('select', player)">
    <span class="avatar">
      <img
        v-if="player.avatar_url && !avatarFailed"
        :src="withApiBase(player.avatar_url, config.public.apiBase)"
        width="32"
        height="32"
        loading="lazy"
        alt=""
        @error="avatarFailed = true"
      >
      <span v-else class="avatar-fallback">{{ initials(props.player.name) }}</span>
    </span>
    <span class="identity">
      <span class="name">{{ player.name }}</span>
      <span class="club">{{ player.club_name }}</span>
    </span>
    <span v-if="player.club_logo_url && !clubLogoFailed" class="club-logo">
      <img
        :src="withApiBase(player.club_logo_url, config.public.apiBase)"
        width="24"
        height="24"
        loading="lazy"
        alt=""
        @error="clubLogoFailed = true"
      >
    </span>
    <span class="rgx">{{ player.rgx }}</span>
  </button>
</template>

<style scoped>
.row {
  width: 100%;
  min-height: 44px;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 20px;
  text-align: left;
}

.avatar {
  width: 32px;
  height: 32px;
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
  font: 600 11px/1 var(--font-display);
  color: var(--text-muted);
}

.identity {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.name {
  font: 500 15px/1.25 var(--font-body);
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.club {
  font: 400 12px/1.3 var(--font-body);
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.club-logo {
  width: 24px;
  height: 24px;
  border-radius: 6px;
  flex: none;
  overflow: hidden;
  background: repeating-linear-gradient(45deg, #242830 0 3px, #1b1e24 3px 6px);
}

.club-logo img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.rgx {
  font: 600 13px/1 var(--font-display);
  color: rgba(244, 244, 242, 0.7);
  min-width: 38px;
  text-align: right;
  font-variant-numeric: tabular-nums;
  flex: none;
}
</style>
