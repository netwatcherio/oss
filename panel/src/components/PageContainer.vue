<script setup lang="ts">
/**
 * PageContainer - Consistent layout wrapper for all pages
 * Provides responsive padding, max-width constraints, and consistent spacing
 */
interface Props {
  /** Size variant: 'narrow' for forms/settings, 'default' for standard pages, 'full' for wide layouts */
  size?: 'narrow' | 'default' | 'full';
  /** Whether to add extra top padding (for pages without Title component) */
  padded?: boolean;
  /** Whether to center the content horizontally */
  centered?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  size: 'default',
  padded: false,
  centered: false,
});
</script>

<template>
  <div 
    class="page-container"
    :class="{
      [`page-container--${size}`]: true,
      'page-container--padded': padded,
      'page-container--centered': centered,
    }"
  >
    <slot />
  </div>
</template>

<style scoped>
.page-container {
  width: 100%;
  margin-left: auto;
  margin-right: auto;
  padding-left: var(--container-padding-x, 1.5rem);
  padding-right: var(--container-padding-x, 1.5rem);
}

/* Size variants */
.page-container--narrow {
  max-width: 640px;
}

.page-container--default {
  max-width: 1400px;
}

.page-container--full {
  max-width: 100%;
}

/* Extra padding for pages without Title */
.page-container--padded {
  padding-top: 2rem;
  padding-bottom: 2rem;
}

/* Center content */
.page-container--centered {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: calc(100vh - 200px);
}

/* Responsive padding adjustments */
@media (max-width: 576px) {
  .page-container {
    padding-left: 1rem;
    padding-right: 1rem;
  }
  
  .page-container--padded {
    padding-top: 1.5rem;
    padding-bottom: 1.5rem;
  }
}

@media (min-width: 1920px) {
  .page-container--default {
    max-width: 1600px;
  }
}
</style>