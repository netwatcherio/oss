import type { ProbeData } from '@/types';

// Safely add a probe data row to an array without duplicates or Vue
// reactivity issues.
//
// Dedup uses a composite key of probe_id + reporting agent + created_at +
// type. agent_id MUST be part of the key: bidirectional probes share one
// probe_id and forward/reverse rows differ only by the reporting agent —
// without it, same-second rows from the two directions silently drop each
// other (seen as gaps in the graphs).
export function addProbeDataUnique(targetArray: ProbeData[], newData: ProbeData) {
  if (!newData) return;

  const newKey = `${newData.probe_id}-${newData.agent_id}-${newData.created_at}-${newData.type}`;
  const exists = targetArray.some(
    (item) => `${item.probe_id}-${item.agent_id}-${item.created_at}-${item.type}` === newKey
  );
  if (exists) return;

  // Assign a stable unique id for Vue reactivity if missing
  if (!newData.id || newData.id === 0) {
    if (typeof crypto !== 'undefined' && (crypto as any).randomUUID) {
      (newData as any).id = (crypto as any).randomUUID();
    } else {
      (newData as any).id = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
        const r = (Math.random() * 16) | 0;
        const v = c === 'x' ? r : (r & 0x3) | 0x8;
        return v.toString(16);
      });
    }
  }

  // Use .push() to preserve reactivity in Vue arrays
  targetArray.push(newData);
}
