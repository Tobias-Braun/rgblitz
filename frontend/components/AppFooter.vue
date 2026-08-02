<script setup lang="ts"></script>

<template>
  <footer class="app-footer">
    <NuxtLink to="/faq" class="footer-link">FAQ</NuxtLink>
    <a href="#" class="footer-link">Datenschutz</a>
    <a href="#" class="footer-link">Impressum</a>
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
