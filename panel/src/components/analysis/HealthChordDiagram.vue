<script lang="ts" setup>
import { ref, onMounted, watch } from 'vue'
import * as d3 from 'd3'
import type { WorkspaceHealthMesh, AgentMeshLink } from './types'

const props = defineProps<{
  mesh: WorkspaceHealthMesh | null
}>()

const svgRef = ref<SVGSVGElement | null>(null)
const empty = ref(false)

// Solid fills for SVG (CSS vars resolve fine in modern browsers, but
// hex keeps the exported/printed SVG self-contained).
const gradeFill: Record<string, string> = {
  excellent: '#2fb344',
  good: '#4b7bec',
  fair: '#f59f00',
  poor: '#f76707',
  critical: '#d63939',
  unknown: '#868e96',
}

function linkTitle(l: AgentMeshLink): string {
  return (
    `${l.source_agent_name} → ${l.target_agent_name}\n` +
    `${l.health.grade} (${Math.round(l.health.overall_health)}/100)\n` +
    `latency ${l.metrics.avg_latency.toFixed(1)} ms · loss ${l.metrics.packet_loss.toFixed(2)}% · ` +
    `jitter ${l.metrics.jitter_avg.toFixed(1)} ms\n` +
    `${l.probe_types.join(' + ')} · ${l.metrics.sample_count} samples`
  )
}

function render() {
  const svg = d3.select(svgRef.value)
  svg.selectAll('*').remove()

  const mesh = props.mesh
  // Only agents that participate in at least one link get an arc.
  const nodes = (mesh?.nodes ?? []).filter((n) => n.link_count > 0)
  const links = mesh?.links ?? []
  empty.value = nodes.length < 2 || links.length === 0
  if (empty.value) return

  const idx = new Map(nodes.map((n, i) => [n.agent_id, i]))
  const matrix: number[][] = Array.from({ length: nodes.length }, () =>
    new Array(nodes.length).fill(0)
  )
  const linkByPair = new Map<string, AgentMeshLink>()
  for (const l of links) {
    const s = idx.get(l.source_agent_id)
    const t = idx.get(l.target_agent_id)
    if (s === undefined || t === undefined) continue
    // sqrt keeps one chatty pair from drowning out the rest; +1 keeps
    // low-sample links visible.
    matrix[s][t] = Math.sqrt(l.metrics.sample_count || 1) + 1
    linkByPair.set(`${s}-${t}`, l)
  }

  const size = 560
  const outer = size / 2 - 90
  const inner = outer - 14

  const chords = d3
    .chordDirected()
    .padAngle(0.05)
    .sortSubgroups(d3.descending)(matrix)

  const g = svg
    .attr('viewBox', `0 0 ${size} ${size}`)
    .append('g')
    .attr('transform', `translate(${size / 2},${size / 2})`)

  const arc = d3.arc<d3.ChordGroup>().innerRadius(inner).outerRadius(outer)
  const ribbon = d3
    .ribbonArrow<d3.Chord, d3.ChordSubgroup>()
    .radius(inner - 2)
    .padAngle(1 / inner)

  const ribbons = g
    .append('g')
    .attr('fill-opacity', 0.72)
    .selectAll('path')
    .data(chords)
    .join('path')
    .attr('d', ribbon as never)
    .attr('fill', (d) => {
      const l = linkByPair.get(`${d.source.index}-${d.target.index}`)
      return gradeFill[l?.health.grade ?? 'unknown']
    })
    .attr('cursor', 'pointer')

  ribbons.append('title').text((d) => {
    const l = linkByPair.get(`${d.source.index}-${d.target.index}`)
    return l ? linkTitle(l) : ''
  })

  const group = g
    .append('g')
    .selectAll('g')
    .data(chords.groups)
    .join('g')

  group
    .append('path')
    .attr('d', arc as never)
    .attr('fill', (d) => gradeFill[nodes[d.index].health.grade] ?? gradeFill.unknown)
    .attr('stroke', 'rgba(0,0,0,0.25)')
    .append('title')
    .text(
      (d) =>
        `${nodes[d.index].agent_name}\n${nodes[d.index].health.grade} ` +
        `(${Math.round(nodes[d.index].health.overall_health)}/100) · ` +
        `${nodes[d.index].link_count} path${nodes[d.index].link_count === 1 ? '' : 's'}`
    )

  group
    .append('text')
    .each((d: d3.ChordGroup & { angle?: number }) => {
      d.angle = (d.startAngle + d.endAngle) / 2
    })
    .attr('dy', '0.35em')
    .attr(
      'transform',
      (d: d3.ChordGroup & { angle?: number }) =>
        `rotate(${((d.angle ?? 0) * 180) / Math.PI - 90}) translate(${outer + 6}) ${
          (d.angle ?? 0) > Math.PI ? 'rotate(180)' : ''
        }`
    )
    .attr('text-anchor', (d: d3.ChordGroup & { angle?: number }) =>
      (d.angle ?? 0) > Math.PI ? 'end' : null
    )
    .attr('class', 'chord-label')
    .text((d) => nodes[d.index].agent_name)

  // Hover: isolate the ribbons touching an agent's arc.
  group
    .on('mouseover', (_evt, d) => {
      ribbons.attr('fill-opacity', (c) =>
        c.source.index === d.index || c.target.index === d.index ? 0.9 : 0.12
      )
    })
    .on('mouseout', () => {
      ribbons.attr('fill-opacity', 0.72)
    })
  ribbons
    .on('mouseover', function () {
      ribbons.attr('fill-opacity', 0.12)
      d3.select(this).attr('fill-opacity', 0.95)
    })
    .on('mouseout', () => {
      ribbons.attr('fill-opacity', 0.72)
    })
}

onMounted(render)
watch(() => props.mesh, render, { deep: true })
</script>

<template>
  <div class="chord-card">
    <div class="chord-header">
      <span class="chord-title">Agent Health Mesh</span>
      <span v-if="mesh && !empty" class="chord-sub">
        {{ mesh.links.length }} paths across {{ mesh.nodes.filter((n) => n.link_count > 0).length }} agents
        — ribbon width = traffic volume, color = path health
      </span>
    </div>
    <div v-if="empty" class="chord-empty">
      No agent-to-agent probe data in this window — the mesh needs at least two agents probing each other.
    </div>
    <svg v-show="!empty" ref="svgRef" class="chord-svg" />
    <div v-if="!empty" class="chord-legend">
      <span v-for="(color, grade) in { excellent: '#2fb344', good: '#4b7bec', fair: '#f59f00', poor: '#f76707', critical: '#d63939' }"
        :key="grade" class="legend-item">
        <span class="legend-swatch" :style="{ background: color }" />{{ grade }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.chord-card {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.chord-header {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.5rem;
}
.chord-title {
  font-weight: 600;
}
.chord-sub {
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
}
.chord-svg {
  width: 100%;
  max-width: 640px;
  margin: 0 auto;
}
.chord-svg :deep(.chord-label) {
  font-size: 11px;
  fill: var(--bs-body-color);
}
.chord-empty {
  color: var(--bs-secondary-color);
  font-size: 0.9rem;
  padding: 1.5rem 0;
  text-align: center;
}
.chord-legend {
  display: flex;
  justify-content: center;
  gap: 1rem;
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
  text-transform: capitalize;
}
.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}
.legend-swatch {
  width: 10px;
  height: 10px;
  border-radius: 2px;
  display: inline-block;
}
</style>
