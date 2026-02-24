<script setup lang="ts">
/**
 * ResponsiveTable - Wraps tables for mobile responsiveness
 * On mobile: horizontal scroll with visual indicator
 * Optional: card-based view for very small screens
 */
interface Props {
  /** Enable card-based mobile view (transforms rows to cards) */
  cardView?: boolean;
  /** Breakpoint at which to switch to card view (if enabled) */
  cardBreakpoint?: number;
  /** Custom card title column index (for card view) */
  cardTitleColumn?: number;
  /** Whether the table has actions column (last column) */
  hasActions?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  cardView: false,
  cardBreakpoint: 576,
  cardTitleColumn: 0,
  hasActions: false,
});
</script>

<template>
  <div 
    class="responsive-table"
    :class="{
      'responsive-table--card-view': cardView,
      'responsive-table--has-actions': hasActions,
    }"
  >
    <!-- Desktop/Tablet: Scrollable Table -->
    <div class="table-wrapper">
      <div class="table-scroll-container">
        <slot />
      </div>
      <!-- Scroll hint for mobile -->
      <div class="scroll-hint d-md-none">
        <i class="bi bi-arrow-left-right"></i>
        <span>Scroll to see more</span>
      </div>
    </div>
    
    <!-- Mobile Card View (optional alternative) -->
    <div v-if="cardView" class="mobile-card-view d-md-none">
      <slot name="mobile-cards" />
    </div>
  </div>
</template>

<style scoped>
.responsive-table {
  width: 100%;
}

/* Table wrapper with horizontal scroll */
.table-wrapper {
  width: 100%;
  position: relative;
}

.table-scroll-container {
  width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: thin;
  scrollbar-color: var(--bs-border-color) transparent;
}

/* Custom scrollbar styling */
.table-scroll-container::-webkit-scrollbar {
  height: 6px;
}

.table-scroll-container::-webkit-scrollbar-track {
  background: transparent;
}

.table-scroll-container::-webkit-scrollbar-thumb {
  background: var(--bs-border-color);
  border-radius: 3px;
}

.table-scroll-container::-webkit-scrollbar-thumb:hover {
  background: var(--bs-secondary-color);
}

/* Ensure table takes full width */
.table-scroll-container :deep(table) {
  width: 100%;
  min-width: 500px; /* Minimum width before scroll */
  margin-bottom: 0;
}

/* Scroll hint */
.scroll-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.5rem;
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  background: linear-gradient(to right, transparent, var(--bs-tertiary-bg), transparent);
  animation: pulse-hint 2s ease-in-out infinite;
}

@keyframes pulse-hint {
  0%, 100% { opacity: 0.6; }
  50% { opacity: 1; }
}

/* Mobile Card View Styles */
.mobile-card-view {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

/* Reduced motion support */
@media (prefers-reduced-motion: reduce) {
  .scroll-hint {
    animation: none;
  }
}

/* Responsive adjustments */
@media (min-width: 768px) {
  .scroll-hint {
    display: none !important;
  }
}

@media (max-width: 767px) {
  .table-scroll-container :deep(table) {
    min-width: 100%;
  }
  
  /* Compact table styling for small screens */
  .table-scroll-container :deep(th),
  .table-scroll-container :deep(td) {
    padding: 0.5rem 0.75rem;
    font-size: 0.875rem;
    white-space: nowrap;
  }
  
  /* Make first column sticky for context */
  .table-scroll-container :deep(th:first-child),
  .table-scroll-container :deep(td:first-child) {
    position: sticky;
    left: 0;
    background: var(--bs-body-bg);
    z-index: 1;
    box-shadow: 2px 0 4px rgba(0,0,0,0.1);
  }
  
  /* Header styling */
  .table-scroll-container :deep(thead th) {
    background: var(--bs-tertiary-bg);
    font-weight: 600;
    text-transform: uppercase;
    font-size: 0.75rem;
    letter-spacing: 0.025em;
  }
}

/* Very small screens - hide table, show cards only */
@media (max-width: 575px) {
  .responsive-table--card-view .table-wrapper {
    display: none;
  }
  
  .mobile-card-view {
    display: flex !important;
  }
}

/* Ensure proper display on larger screens */
@media (min-width: 576px) {
  .mobile-card-view {
    display: none !important;
  }
}

/* Table row hover enhancement */
.table-scroll-container :deep(tbody tr) {
  transition: background-color 0.15s ease;
}

.table-scroll-container :deep(tbody tr:hover) {
  background-color: var(--bs-tertiary-bg);
}

/* Action buttons column styling */
.responsive-table--has-actions :deep(td:last-child),
.responsive-table--has-actions :deep(th:last-child) {
  text-align: right;
  min-width: 120px;
}

/* Status badges in tables */
.table-scroll-container :deep(.badge) {
  font-size: 0.75rem;
  padding: 0.35em 0.65em;
}

/* Empty state within table */
.table-scroll-container :deep(.table-empty) {
  text-align: center;
  padding: 3rem 1rem;
  color: var(--bs-secondary-color);
}

.table-scroll-container :deep(.table-empty i) {
  font-size: 2rem;
  margin-bottom: 0.5rem;
  display: block;
}
</style>