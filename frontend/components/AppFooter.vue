<script setup lang="ts"></script>

<template>
  <footer class="app-footer">
    <a href="#" class="footer-link">FAQ</a>
    <a href="#" class="footer-link">Datenschutz</a>
    <a href="#" class="footer-link">Impressum</a>
  </footer>
</template>

<style scoped>
.app-footer {
  position: sticky;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 10;
  display: flex;
  justify-content: center;
  gap: 18px;
  width: 100%;
  padding: 8px 18px;
  background: var(--bg);
  border-top: 1px solid var(--border);
}

.footer-link {
  font: 400 11px/1 var(--font-body);
  color: var(--text-dim);
  text-decoration: none;
}

.footer-link:hover {
  color: var(--text-muted);
}
</style>
