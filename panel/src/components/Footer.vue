<script lang="ts" setup>
const currentYear = new Date().getFullYear();
</script>

<template>
  <footer class="app-footer">
    <div class="footer-content">
      <span class="footer-text">
        netwatcher.io © 2022-{{ currentYear }}
      </span>
    </div>
  </footer>
</template>

<style scoped>
.app-footer {
  width: 100%;
  padding: 1rem 0;
  text-align: center;
  background-color: transparent;
  border-top: 1px solid rgba(55, 65, 81, 0.5);
  margin-top: auto;
}

.footer-content {
  max-width: 1400px;
  margin: 0 auto;
  padding: 0 1.5rem;
}

.footer-text {
  font-size: 0.875rem;
  color: #6b7280;
  letter-spacing: 0.025em;
}

/* Dark mode styles */
:global([data-theme="dark"]) .app-footer {
  background-color: transparent;
  border-top-color: rgba(55, 65, 81, 0.5);
}

:global([data-theme="dark"]) .footer-text {
  color: #6b7280;
}

/* Light mode styles */
:global([data-theme="light"]) .app-footer {
  background-color: transparent;
  border-top-color: rgba(209, 213, 219, 0.5);
}

:global([data-theme="light"]) .footer-text {
  color: #6b7280;
}
</style>