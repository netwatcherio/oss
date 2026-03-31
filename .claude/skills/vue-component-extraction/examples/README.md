# Vue Component Extraction Examples

This directory contains real-world examples from the Agent.vue refactor.

## Available Examples

### 1. Header Extraction (`header-extraction.md`)
Shows how to extract a header component with:
- Title/breadcrumb navigation
- Status badges with icons
- Action buttons (share, edit, add)
- Event emissions to parent

### 2. Stats Bar Extraction (`stats-bar-extraction.md`)
Demonstrates extracting complex UI with:
- Circular progress rings (SVG)
- Loading skeleton states
- Color-coded status indicators
- Computed calculations in component

### 3. Tab Extraction (`tab-extraction.md`)
The most complex example showing:
- Multiple tab components
- Moving helper functions from parent
- Shared state passed as props
- Massive file size reduction

## Pattern Summary

Each example follows the same structure:

1. **Before** - Show the code in the original large component
2. **After** - The extracted component with props/emits
3. **Parent Usage** - How to use the component
4. **Key Points** - What to watch out for

## Common Props Patterns

### State Objects
```typescript
// ❌ Bad - passing entire state
<ChildComponent :state="state" />

// ✅ Good - passing specific values
<ChildComponent 
  :agent-name="state.agent.name"
  :workspace-id="state.workspace.id"
/>
```

### Loading States
```typescript
// ❌ Bad - passing whole loadingState
<ChildComponent :loading-state="loadingState" />

// ✅ Good - boolean flags
<ChildComponent 
  :is-loading="loadingState.agent"
  :is-data-loading="loadingState.data"
/>
```

### Events
```typescript
// ❌ Bad - passing callbacks
<ChildComponent :on-share="showShareModal" />

// ✅ Good - using emits
<ChildComponent @share="showShareModal = true" />
```

## Metrics from Real Refactor

- **Original file:** 3,632 lines
- **Final file:** 2,843 lines (-21.7%)
- **Components created:** 6
- **Functions removed from parent:** 8
- **Lines moved to components:** ~1,590
