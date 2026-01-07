/**
 * Probe Grouping Utilities
 *
 * This module provides functions for organizing and matching probes by their targets.
 *
 * ## Target Types
 * - **host**: Probe targets an IP/hostname (e.g., "8.8.8.8")
 * - **agent**: Probe targets another agent by ID (e.g., agent_id: 101)
 * - **local**: Probe has no external targets (self-monitoring)
 *
 * ## Key Functions
 * - `groupProbesByTarget()` - Group all probes by target for summary views
 * - `findMatchingProbesByProbeId()` - Find probes with same target as given probe
 * - `findProbesByInitialTarget()` - Find probes sharing first target (for Probe.vue)
 * - `canonicalTargetKey()` - Create stable key like "agent:101" or "host:8.8.8.8"
 *
 * ## Controller Integration
 * When a probe targets an agent (not a host), the controller resolves the IP dynamically
 * at fetch time using `getPublicIP()`. The panel receives the filled `target` field.
 *
 * @module probeGrouping
 * @see docs/panel-architecture.md
 */
import type { Probe, ProbeType, Target } from "@/types";

export type TargetGroupKind = "host" | "agent" | "local";

interface PerTypeStats {
    probes: Probe[];
    count: number;
    enabled: number;
    intervals: number[];
}

export interface ProbeGroupByTarget {
    key: string;                 // `${kind}|${id}`
    kind: TargetGroupKind;
    id: string | number;
    label: string;

    types: ProbeType[];
    countProbes: number;
    countEnabled: number;
    countTargets: number;
    firstSeen: string;
    lastSeen: string;
    anyServer: boolean;

    perType: Record<string, PerTypeStats>;
    probes: Probe[];
    targets: Target[];
}

interface TypeTotals {
    probes: number;
    enabled: number;
    targets: number;
}

export interface GroupingResult {
    groups: ProbeGroupByTarget[];
    byKey: Record<string, ProbeGroupByTarget>;
    kinds: TargetGroupKind[];
    totals: {
        probes: number;
        enabled: number;
        targets: number;
        byType: Record<string, TypeTotals>;
    };
}

type GroupOptions = {
    /** If true, exclude default “built-in” types:
     * SPEEDTEST, NETINFO, SPEEDTEST_SERVERS, SYSINFO
     */
    excludeDefaults?: boolean;
    /** Custom list of types to exclude (case-insensitive if caseInsensitive = true) */
    excludeTypes?: string[] | Set<string>;
    /** Treat type matching case-insensitively (default true) */
    caseInsensitive?: boolean;
};

const DEFAULT_EXCLUDE = new Set(["SPEEDTEST", "NETINFO", "SPEEDTEST_SERVERS", "SYSINFO"]);

const normHost = (h: string) => h.trim().toLowerCase();
const isEmptyStr = (s?: string | null) => !s || s.trim() === "";
const isEmptyAgentId = (id?: number | null) => id == null || id === 0;
const getProbeAgentId = (p: Probe) => (p.agent_id ?? (p as any).agent_id ?? 0);
const getTargetAgentId = (t: Target) => (t.agent_id ?? (t as any).agent_id ?? null);
const makeKey = (kind: TargetGroupKind, id: string | number) => `${kind}|${String(id)}`;

function buildExclusionSet(opts?: GroupOptions): Set<string> {
    const caseInsensitive = opts?.caseInsensitive ?? true;

    const set = new Set<string>();
    if (opts?.excludeDefaults) {
        for (const t of DEFAULT_EXCLUDE) set.add(caseInsensitive ? t.toUpperCase() : t);
    }
    if (opts?.excludeTypes) {
        const list = Array.isArray(opts.excludeTypes)
            ? opts.excludeTypes
            : Array.from(opts.excludeTypes.values());
        for (const t of list) set.add(caseInsensitive ? t.toUpperCase() : t);
    }
    return set;
}

function shouldSkip(type: string, excludeSet: Set<string>, caseInsensitive: boolean): boolean {
    const key = caseInsensitive ? type.toUpperCase() : type;
    return excludeSet.has(key);
}

/** Group across TYPES by TARGET; optionally exclude some probe types. */
export function groupProbesByTarget(
    probes: Probe[],
    opts?: GroupOptions
): GroupingResult {
    type Acc = {
        kind: TargetGroupKind;
        id: string | number;
        label: string;

        probesSet: Set<number>;
        enabledSet: Set<number>;
        targets: Target[];
        anyServer: boolean;
        first: string | null;
        last: string | null;

        perType: Map<
            string,
            { probeIds: Set<number>; enabledIds: Set<number>; intervals: Set<number>; probes: Probe[] }
        >;
    };

    const caseInsensitive = opts?.caseInsensitive ?? true;
    const excludeSet = buildExclusionSet(opts);

    const groups = new Map<string, Acc>();
    const kindsSet = new Set<TargetGroupKind>();

    // Overall totals (dedup)
    const allProbeIds = new Set<number>();
    const allEnabledIds = new Set<number>();
    let totalTargets = 0;

    // Per-type totals
    const typeProbeIds = new Map<string, Set<number>>();
    const typeEnabledProbeIds = new Map<string, Set<number>>();
    const typeTargets = new Map<string, number>();

    const addToTypeTotals = (ptype: string, pid: number, enabled: boolean, addTargets = 0) => {
        if (!typeProbeIds.has(ptype)) typeProbeIds.set(ptype, new Set());
        if (!typeEnabledProbeIds.has(ptype)) typeEnabledProbeIds.set(ptype, new Set());
        if (!typeTargets.has(ptype)) typeTargets.set(ptype, 0);

        typeProbeIds.get(ptype)!.add(pid);
        if (enabled) typeEnabledProbeIds.get(ptype)!.add(pid);
        if (addTargets) typeTargets.set(ptype, (typeTargets.get(ptype) || 0) + addTargets);
    };

    // 1) target-backed groups (host/agent)
    for (const p of probes) {
        if (shouldSkip(p.type, excludeSet, caseInsensitive)) continue;

        const pEnabled = !!p.enabled;
        const pid = p.id;

        // global presence
        allProbeIds.add(pid);
        if (pEnabled) allEnabledIds.add(pid);
        addToTypeTotals(p.type, pid, pEnabled, 0);

        const tlist = p.targets ?? [];
        if (tlist.length > 0) {
            for (const t of tlist) {
                const hasHost = !isEmptyStr(t.target);
                const tgtAid = getTargetAgentId(t);
                const hasAgent = !isEmptyAgentId(tgtAid);

                let kind: TargetGroupKind | null = null;
                let id: string | number | null = null;
                let label = "";

                if (hasHost && !hasAgent) {
                    kind = "host";
                    id = normHost(t.target);
                    label = t.target.trim();
                } else if (!hasHost && hasAgent) {
                    kind = "agent";
                    id = tgtAid as number;
                    label = `Agent #${id}`;
                } else {
                    continue; // ambiguous target row
                }

                const key = makeKey(kind, id);
                if (!groups.has(key)) {
                    groups.set(key, {
                        kind,
                        id,
                        label,
                        probesSet: new Set<number>(),
                        enabledSet: new Set<number>(),
                        targets: [],
                        anyServer: false,
                        first: null,
                        last: null,
                        perType: new Map(),
                    });
                    kindsSet.add(kind);
                }

                const acc = groups.get(key)!;

                if (!acc.probesSet.has(pid)) {
                    acc.probesSet.add(pid);
                    if (pEnabled) acc.enabledSet.add(pid);
                    if (p.server) acc.anyServer = true;

                    acc.first =
                        acc.first == null || new Date(p.created_at) < new Date(acc.first)
                            ? p.created_at
                            : acc.first;
                    acc.last =
                        acc.last == null || new Date(p.updated_at) > new Date(acc.last)
                            ? p.updated_at
                            : acc.last;
                }

                acc.targets.push(t);
                totalTargets += 1;
                addToTypeTotals(p.type, pid, pEnabled, 1);

                if (!acc.perType.has(p.type)) {
                    acc.perType.set(p.type, {
                        probeIds: new Set<number>(),
                        enabledIds: new Set<number>(),
                        intervals: new Set<number>(),
                        probes: [],
                    });
                }
                const pt = acc.perType.get(p.type)!;
                if (!pt.probeIds.has(pid)) {
                    pt.probeIds.add(pid);
                    if (pEnabled) pt.enabledIds.add(pid);
                    pt.intervals.add(p.interval_sec);
                    pt.probes.push(p);
                }
            }
        }
    }

    // 2) local groups (no targets)
    for (const p of probes) {
        if (shouldSkip(p.type, excludeSet, caseInsensitive)) continue;
        if ((p.targets ?? []).length > 0) continue;

        const pEnabled = !!p.enabled;
        const pid = p.id;
        const aid = getProbeAgentId(p) || 0;
        const kind: TargetGroupKind = "local";
        const id = aid;
        const label = `Local on Agent #${aid}`;

        // global presence
        allProbeIds.add(pid);
        if (pEnabled) allEnabledIds.add(pid);
        addToTypeTotals(p.type, pid, pEnabled, 0);

        const key = makeKey(kind, id);
        if (!groups.has(key)) {
            groups.set(key, {
                kind,
                id,
                label,
                probesSet: new Set<number>(),
                enabledSet: new Set<number>(),
                targets: [],
                anyServer: false,
                first: null,
                last: null,
                perType: new Map(),
            });
            kindsSet.add(kind);
        }

        const acc = groups.get(key)!;

        if (!acc.probesSet.has(pid)) {
            acc.probesSet.add(pid);
            if (pEnabled) acc.enabledSet.add(pid);
            if (p.server) acc.anyServer = true;

            acc.first =
                acc.first == null || new Date(p.created_at) < new Date(acc.first)
                    ? p.created_at
                    : acc.first;
            acc.last =
                acc.last == null || new Date(p.updated_at) > new Date(acc.last)
                    ? p.updated_at
                    : acc.last;
        }

        if (!acc.perType.has(p.type)) {
            acc.perType.set(p.type, {
                probeIds: new Set<number>(),
                enabledIds: new Set<number>(),
                intervals: new Set<number>(),
                probes: [],
            });
        }
        const pt = acc.perType.get(p.type)!;
        if (!pt.probeIds.has(pid)) {
            pt.probeIds.add(pid);
            if (pEnabled) pt.enabledIds.add(pid);
            pt.intervals.add(p.interval_sec);
            pt.probes.push(p);
        }
    }

    // 3) finalize
    const groupsOut: ProbeGroupByTarget[] = [];
    const byKey: Record<string, ProbeGroupByTarget> = {};

    for (const [key, acc] of groups) {
        const perType: Record<string, PerTypeStats> = {};
        const types: string[] = [];

        for (const [t, v] of acc.perType.entries()) {
            types.push(t);
            perType[t] = {
                probes: v.probes,
                count: v.probeIds.size,
                enabled: v.enabledIds.size,
                intervals: Array.from(v.intervals.values()).sort((a, b) => a - b),
            };
        }

        types.sort();

        const group: ProbeGroupByTarget = {
            key,
            kind: acc.kind,
            id: acc.id,
            label: acc.label,
            types,
            countProbes: acc.probesSet.size,
            countEnabled: acc.enabledSet.size,
            countTargets: acc.targets.length,
            firstSeen: acc.first ?? "",
            lastSeen: acc.last ?? "",
            anyServer: acc.anyServer,
            perType,
            probes: Array.from(acc.perType.values())
                .flatMap(v => v.probes)
                .filter((p, i, arr) => arr.findIndex(pp => pp.id === p.id) === i),
            targets: acc.targets,
        };

        groupsOut.push(group);
        byKey[key] = group;
    }

    const kindOrder: Record<TargetGroupKind, number> = { host: 0, agent: 1, local: 2 };
    groupsOut.sort((a, b) => (a.kind !== b.kind ? kindOrder[a.kind] - kindOrder[b.kind] : String(a.label).localeCompare(String(b.label))));

    // totals.byType
    const totalsByType: Record<string, TypeTotals> = {};
    for (const [t, ids] of typeProbeIds.entries()) {
        totalsByType[t] = {
            probes: ids.size,
            enabled: typeEnabledProbeIds.get(t)?.size ?? 0,
            targets: typeTargets.get(t) ?? 0,
        };
    }

    return {
        groups: groupsOut,
        byKey,
        kinds: Array.from(kindsSet.values()),
        totals: {
            probes: allProbeIds.size,
            enabled: allEnabledIds.size,
            targets: totalTargets,
            byType: totalsByType,
        },
    };
}

type SimilarMatches = {
    /** Probes that match any host-based target of the subject probe */
    byHost: Probe[];
    /** Probes that match any agent-id-based target of the subject probe */
    byAgent: Probe[];
    /** Union of both (deduped), excluding the subject */
    combined: Probe[];
    /** Which group keys we matched (debug/telemetry) */
    matchedKeys: string[];
};

function uniqueById<T extends { id: number }>(arr: T[]): T[] {
    const seen = new Set<number>();
    const out: T[] = [];
    for (const x of arr) if (!seen.has(x.id)) { seen.add(x.id); out.push(x); }
    return out;
}

/** Reuse your helpers */
const keyForHost = (host: string) => makeKey("host", normHost(host));
const keyForAgent = (aid: number) => makeKey("agent", aid);

/** Build the group keys that a single probe participates in (host|agent|local). */
function keysForProbeTargetsStrict(p: Probe): string[] {
    const keys: string[] = [];
    const tlist = p.targets ?? [];

    for (const t of tlist) {
        const hasHost = !!t.target && t.target.trim() !== "";
        const tgtAid = getTargetAgentId(t);
        const hasAgent = tgtAid != null && tgtAid !== 0;

        // Match your grouper’s logic: one or the other, not both
        if (hasHost && !hasAgent) keys.push(keyForHost(t.target!.trim()));
        else if (!hasHost && hasAgent) keys.push(keyForAgent(Number(tgtAid)));
    }

    // If no targets, treat as local on probe’s own agent (your grouper does this)
    if (keys.length === 0) {
        const aid = getProbeAgentId(p) || 0;
        keys.push(makeKey("local", aid));
    }

    return Array.from(new Set(keys));
}

/**
 * Find probes that share any target (host or agent) with the given probeId.
 * - If you already computed `grouped`, pass it to avoid re-grouping.
 * - Options:
 *   - sameTypeOnly: only return matches with the same Probe.type (default true)
 *   - excludeDefaults: forward to groupProbesByTarget to skip built-ins (default true)
 */
export function findMatchingProbesByProbeId(
    probeId: number,
    allProbes: Probe[],
    grouped?: GroupingResult,
    opts?: { sameTypeOnly?: boolean; excludeDefaults?: boolean; caseInsensitive?: boolean }
): SimilarMatches {
    const sameTypeOnly = opts?.sameTypeOnly ?? false;

    const subject = allProbes.find(p => p.id === probeId);
    if (!subject) {
        return { byHost: [], byAgent: [], combined: [], matchedKeys: [] };
    }

    const g = grouped ?? groupProbesByTarget(allProbes, {
        excludeDefaults: opts?.excludeDefaults ?? true,
        caseInsensitive: opts?.caseInsensitive ?? true,
    });

    const subjectKeys = keysForProbeTargetsStrict(subject);

    // Collect matches per key
    const hostMatches: Probe[] = [];
    const agentMatches: Probe[] = [];
    const matchedKeys: string[] = [];

    for (const key of subjectKeys) {
        const grp = g.byKey[key];
        if (!grp) continue;

        // grp.probes already deduped per your grouper
        let candidates = grp.probes.filter(p => p.id !== subject.id);
        if (sameTypeOnly) candidates = candidates.filter(p => p.type === subject.type);

        if (key.startsWith("host|")) hostMatches.push(...candidates);
        else if (key.startsWith("agent|")) agentMatches.push(...candidates);
        // local|… will fall into neither byHost nor byAgent; they’ll still be in combined below

        matchedKeys.push(key);
    }

    const combined = uniqueById([...hostMatches, ...agentMatches]);

    // Optional nice ordering: enabled first, then type, then labelish thing
    const sortFn = (a: Probe, b: Probe) =>
        Number(!!b.enabled) - Number(!!a.enabled) ||
        String(a.type).localeCompare(String(b.type)) ||
        String(a.id).localeCompare(String(b.id));

    return {
        byHost: uniqueById(hostMatches).sort(sortFn),
        byAgent: uniqueById(agentMatches).sort(sortFn),
        combined: combined.sort(sortFn),
        matchedKeys: matchedKeys,
    };
}

export function findProbesByInitialTarget(seed: Probe, all: Probe[]): Probe[] {
    const seedKey = canonicalTargetKey(seed);
    if (!seedKey) return [];

    return all.filter(p => canonicalTargetKey(p) === seedKey);
}

/** Builds a stable key from a probe's *first* target.
 *  Prefer agentId; otherwise use the target string (lowercased & trimmed). */
function canonicalTargetKey(probe: Probe): string | null {
    if (!probe?.targets?.length) return null;

    const t = probe.targets[0];
    // Prefer agentId if set (including 0 as a valid id)
    if (t && t.agent_id !== undefined && t.agent_id !== null) {
        return `agent:${t.agent_id}`;
    }

    // Fallback to target string
    const s = (t?.target ?? "").trim();
    if (!s) return null;

    // Minimal normalization: lowercase, strip trailing dot and extra spaces
    // (Keep port and IPv6 brackets as part of the identity on purpose.)
    const normalized = s.toLowerCase().replace(/\.+$/, "");
    return `host:${normalized}`;
}