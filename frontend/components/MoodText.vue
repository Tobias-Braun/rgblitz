<!-- Threshold-based German phrase below the RGX circle (docs/02 §3b, config/mood-phrases.ts). -->
<script setup lang="ts">
import { computed } from "vue";
import { moodTextFor } from "~/config/mood-phrases";

const props = defineProps<{
  totalDelta: number;
}>();

const phrase = computed(() => moodTextFor(props.totalDelta));
</script>

<template>
  <p class="mood-text">{{ phrase }}</p>
</template>

<style scoped>
.mood-text {
  margin: 0 16px;
  font: 400 15px/1.5 var(--font-body);
  color: var(--text-muted);
  text-align: center;
  text-wrap: pretty;
}
</style>
