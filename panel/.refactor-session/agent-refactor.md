# Agent.vue Refactor Session

## Goal
Break down 3,632-line Agent.vue into maintainable components while preserving all functionality.

## Phases

### Phase 1: Extract 3 Critical Components (Current)
Priority: Components with most template code duplication

1. **AgentHeader.vue** - Title bar, status badges, action buttons
   - Lines to extract: ~50 from template
   - Props: agent, workspace, permissions, wsConnected
   - Emits: share, edit probes, add probe

2. **QuickStatsBar.vue** - Circular progress rings
   - Lines to extract: ~120 from template
   - Props: loadingState, systemInfo, systemData, probes, targetGroups
   - Heavy SCSS: progress rings, status colors

3. **UninitializedState.vue** - PIN display + install commands
   - Lines to extract: ~60 from template
   - Props: agent, pendingPin, loadingPendingPin
   - Contains: Linux/Windows/Docker command generation

### Phase 2: Extract Tab Components
- OverviewTab.vue (network info, interfaces)
- ProbesTab.vue (probe grid)
- SystemTab.vue (resources, system info)

### Phase 3: Polish & Shared Styles
- Move reusable styles to agent-base.scss
- Extract composables (useProbeStats, useInstallCommands)

## Progress Log

### 2025-03-30
- [ ] Create AgentHeader.vue
- [ ] Create QuickStatsBar.vue  
- [ ] Create UninitializedState.vue
- [ ] Update Agent.vue to use new components
- [ ] Verify all functionality preserved

## SKILLS File Created
- `.refactor-session/SKILL-vue-component-extraction.md`

## Metrics
- Original: 3,632 lines
- Target: <800 lines (Phase 1), <400 lines (Phase 3)
- Components to create: 9 total
- Estimated reduction: 80% line count in Agent.vue
