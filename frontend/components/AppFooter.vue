<script setup lang="ts"></script>

<template>
  <footer class="app-footer">
    <NuxtLink to="/faq" class="footer-link">FAQ</NuxtLink>
    <NuxtLink to="/datenschutz" class="footer-link">Datenschutz</NuxtLink>
    <NuxtLink to="/impressum" class="footer-link">Impressum</NuxtLink>
  </footer>
</template>

<style scoped>
.app-footer {
  flex: none;
  display: flex;
  justify-content: center;
  gap: 18px;
  width: 100%;
  padding: 8px 18px;
  background: var(--bg);
  border-top: 1px solid var(--border);
}

.footer-link {
  font: 400 14px/1 var(--font-body);
  color: var(--text-dim);
  text-decoration: none;
}

.footer-link:hover {
  color: var(--text-muted);
}
</style>
