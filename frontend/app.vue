<template>
  <div class="app-shell">
    <NuxtPage />
    <AppFooter />
  </div>
</template>

<style scoped>
.app-shell {
  min-height: 100dvh;
  display: flex;
  flex-direction: column;
  background: linear-gradient(
    180deg,
    var(--bg) 0%,
    var(--surface) 50%,
    var(--bg) 100%
  );
}
</style>
