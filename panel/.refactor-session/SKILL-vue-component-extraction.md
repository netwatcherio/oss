# SKILL: Vue Component Extraction

## When to Use
- Vue files >500 lines
- Repeated template patterns
- Mixed concerns (data + UI + logic)

## Steps

### 1. Identify Component Boundaries
```
Look for:
- [ ] Self-contained UI section
- [ ] Clear inputs (props) and outputs (emits)
- [ ] No parent state mutations
- [ ] Can be tested in isolation
```

### 2. Create Component File
```vue
<!-- components/[ComponentName].vue -->
<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  // Define all inputs
}

const props = defineProps<Props>()
const emit = defineEmits<{
  // Define all outputs
}>()

// Component logic
</script>

<template>
  <!-- Extracted template -->
</template>

<style scoped>
/* Component styles */
</style>
```

### 3. Extract Props
- Find all `state.*` references → convert to props
- Find all `loadingState.*` → pass as loading booleans
- Find event handlers → emit events to parent

### 4. Migrate Styles
- Copy relevant `<style scoped>` blocks
- Move shared utilities to parent or base file
- Keep animations/keyframes in component if specific

### 5. Update Parent
```typescript
// Replace template section with:
<ComponentName
  :prop1="state.value"
  :prop2="computedValue"
  @event="handleEvent"
/>
```

### 6. Verify Checklist
- [ ] TypeScript types preserved
- [ ] All props have defaults where needed
- [ ] Loading states work
- [ ] Events bubble correctly
- [ ] Styles match exactly
- [ ] No console errors

## Common Patterns

### Loading State Wrapper
```vue
<div :class="{ 'loading': isLoading }">
  <template v-if="!isLoading">...</template>
  <template v-else>...</template>
</div>
```

### Copy-to-Clipboard Pattern
```typescript
const copiedField = ref<string | null>(null)
async function copy(text: string, field: string) {
  await navigator.clipboard.writeText(text)
  copiedField.value = field
  setTimeout(() => copiedField.value = null, 2000)
}
```

### Status Badge Pattern
```vue
<div class="status-badge" :class="statusClass">
  <i :class="statusIcon"></i>
  {{ statusLabel }}
</div>
```

## Anti-Patterns to Avoid
- ❌ Passing entire state object as prop
- ❌ Emitting state mutations directly
- ❌ Copying all parent styles (only what's needed)
- ❌ Losing TypeScript strictness
