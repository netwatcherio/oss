# SKILL: Vue Component Extraction

## Description
Extract large Vue components into smaller, maintainable sub-components.

## When to Apply
- Vue files >500 lines
- Repeated template patterns
- Mixed concerns (data + UI + logic)
- Need better testability

## Prerequisites
- Vue 3 with TypeScript
- Props/Emits understanding
- Component-scoped styles

## Steps

### 1. Identify Component Boundaries
Look for:
- Self-contained UI section
- Clear inputs (props) and outputs (emits)
- No parent state mutations
- Can be tested in isolation

### 2. Analyze Template Section
Find all:
- `state.*` references → convert to props
- `loadingState.*` → pass as loading booleans
- Computed property dependencies
- Event handlers → emit events
- Copy-to-clipboard patterns
- Helper functions → move to component or keep shared

### 3. Create Component File

```vue
<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  // All external data
}

const props = defineProps<Props>()
const emit = defineEmits<{
  // All event outputs
}>()

// Component logic
</script>

<template>
  <!-- Extracted template -->
</template>

<style scoped>
/* Component-specific styles */
</style>
```

### 4. Extract Props

**Pattern 1: Direct State Mapping**
```typescript
// Before (in parent)
state.agent.name
state.workspace.id

// After (props)
interface Props {
  agentName: string
  workspaceId: string | number
}
```

**Pattern 2: Loading States**
```typescript
// Pass booleans instead of whole state
interface Props {
  isLoading: boolean
  isAgentLoading: boolean
}
```

**Pattern 3: Permission Guards**
```typescript
interface Props {
  canEdit: boolean
  canShare: boolean
}
```

### 5. Migrate Event Handlers

```typescript
// Before
@click="showShareModal = true"

// After
const emit = defineEmits<{
  share: []
}>()

@click="emit('share')"
```

### 6. Migrate Styles

- Copy relevant `<style scoped>` blocks
- Remove parent-specific selectors
- Keep animations/keyframes if component-specific
- Move shared utilities to parent or `@import`

### 7. Update Parent Component

```vue
<script setup>
import ChildComponent from '@/components/ChildComponent.vue'
</script>

<template>
  <ChildComponent
    :agent-name="state.agent.name"
    :workspace-id="state.workspace.id"
    :is-loading="loadingState.agent"
    @share="showShareModal = true"
  />
</template>
```

## Common Patterns

### Loading State Wrapper
```vue
<div :class="{ 'skeleton': isLoading }">
  <template v-if="!isLoading">{{ value }}</template>
  <template v-else>
    <span class="skeleton-text">--</span>
  </template>
</div>
```

### Copy-to-Clipboard
```typescript
const copiedField = ref<string | null>(null)

async function copyToClipboard(text: string, field: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedField.value = field
    setTimeout(() => copiedField.value = null, 2000)
  } catch (err) {
    console.error('Failed to copy:', err)
  }
}
```

### Status Badge
```vue
<div class="status-badge" :class="status">
  <i :class="getStatusIcon(status)"></i>
  {{ getStatusLabel(status) }}
</div>
```

### Circular Progress Ring
```vue
<svg class="progress-ring" width="68" height="68">
  <circle class="progress-ring-bg" r="28" cx="34" cy="34" />
  <circle
    class="progress-ring-fill"
    :style="{ 
      strokeDasharray: circumference, 
      strokeDashoffset: offset 
    }"
    r="28" cx="34" cy="34"
  />
</svg>
```

## Anti-Patterns

- ❌ Passing entire reactive state object as prop
- ❌ Emitting state mutations (use events)
- ❌ Copying all parent styles
- ❌ Losing TypeScript strictness
- ❌ Creating circular dependencies

## Troubleshooting

### Component doesn't render
- Check all props are being passed correctly
- Verify types match between parent and component
- Look for console errors about missing props

### Styles not applying
- Ensure `scoped` attribute on style tag
- Check for parent-specific selectors
- Verify CSS specificity isn't being overridden

### TypeScript errors
- Define explicit interface for Props
- Use `defineProps<Props>()` not just `defineProps()`
- Check for any `any` types in the component

### Events not working
- Verify `defineEmits` is called
- Check emit names match between component and parent
- Ensure parent has handler function defined

## Verification Checklist

- [ ] TypeScript compilation passes
- [ ] All props have proper types
- [ ] Loading states preserved
- [ ] Events work correctly
- [ ] Styles match exactly
- [ ] No console warnings
- [ ] Component renders in isolation

## Examples

See `.claude/skills/vue-component-extraction/examples/` for:
- Header extraction
- Stats bar extraction  
- Form section extraction
- Table extraction
