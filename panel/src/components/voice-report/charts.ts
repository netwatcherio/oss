// panel/src/components/voice-report/charts.ts
//
// Lightweight, dependency-free SVG chart helpers for the voice report
// view. Print-safe (no Canvas, no ApexCharts), drop-in for the
// browser's window.print() PDF flow.
//
// Mirrors the SVG charts in the static templates
// (`panel/template/voice-quality-report.html`) so the on-screen view
// renders the same way the printed page does.

export interface LineSeries {
  name: string
  data: number[]
  color: string
  fill?: boolean
}

export interface LineChartOpts {
  height?: number
  durationSec?: number
  hline?: number | null
  yMin?: number
  yMax?: number
  yLabelFormatter?: (v: number) => string
  xLabelFormatter?: (sec: number) => string
}

const defaultXFormatter = (sec: number) => {
  const mm = String(Math.floor(sec / 60)).padStart(2, '0')
  const ss = String(sec % 60).padStart(2, '0')
  return `${mm}:${ss}`
}

export function lineChartSVG(series: LineSeries[], opts: LineChartOpts = {}): string {
  const W = 660
  const H = opts.height ?? 130
  const PL = 34
  const PB = 16
  const PT = 6
  const PR = 6
  const iw = W - PL - PR
  const ih = H - PT - PB
  const all = series.flatMap((s) => s.data)
  if (opts.hline != null) all.push(opts.hline)
  if (all.length === 0) return ''
  const yMax =
    opts.yMax != null
      ? opts.yMax
      : Math.ceil((Math.max(...all) * 1.15) / 10) * 10 || 10
  const yMin = opts.yMin ?? 0
  const n = series[0]?.data.length ?? 0
  const x = (i: number) => PL + ((n > 1 ? i / (n - 1) : 0.5) * iw)
  const y = (v: number) => PT + ih - ((v - yMin) / (yMax - yMin)) * ih

  let g = ''
  // Gridlines
  for (let t = 0; t <= 4; t++) {
    const v = yMin + ((yMax - yMin) / 4) * t
    const yy = y(v)
    g += `<line x1="${PL}" y1="${yy.toFixed(1)}" x2="${(W - PR).toFixed(1)}" y2="${yy.toFixed(1)}" stroke="#eef2f7" stroke-width="1"/>`
    g += `<text x="${(PL - 5).toFixed(1)}" y="${(yy + 3).toFixed(1)}" font-size="7" fill="#94a3b8" text-anchor="end">${
      opts.yLabelFormatter ? opts.yLabelFormatter(v) : v.toFixed(0)
    }</text>`
  }
  // X labels
  const dur = opts.durationSec ?? n
  const xf = opts.xLabelFormatter ?? defaultXFormatter
  ;[0, 0.25, 0.5, 0.75, 1].forEach((f) => {
    const sec = Math.round(dur * f)
    g += `<text x="${(PL + iw * f).toFixed(1)}" y="${(H - 3).toFixed(1)}" font-size="7" fill="#94a3b8" text-anchor="middle">${xf(sec)}</text>`
  })
  if (opts.hline != null) {
    const yy = y(opts.hline)
    g += `<line x1="${PL}" y1="${yy.toFixed(1)}" x2="${(W - PR).toFixed(1)}" y2="${yy.toFixed(1)}" stroke="#ef4444" stroke-width="1" stroke-dasharray="4 3"/>`
  }
  series.forEach((s) => {
    const pts = s.data.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ')
    if (s.fill) {
      g += `<polygon points="${PL},${y(0).toFixed(1)} ${pts} ${(W - PR).toFixed(1)},${y(0).toFixed(1)}" fill="${s.color}" opacity="0.08"/>`
    }
    g += `<polyline points="${pts}" fill="none" stroke="${s.color}" stroke-width="1.6" stroke-linejoin="round"/>`
  })
  return `<svg viewBox="0 0 ${W} ${H}" style="width:100%;height:auto" xmlns="http://www.w3.org/2000/svg">${g}</svg>`
}

export interface BarDatum {
  label: string
  v: number
  color?: string
}

export function barChartSVG(data: BarDatum[], opts: { height?: number } = {}): string {
  const W = 660
  const H = opts.height ?? 130
  const PL = 34
  const PB = 26
  const PT = 6
  const PR = 6
  const iw = W - PL - PR
  const ih = H - PT - PB
  if (data.length === 0) return ''
  const yMax = Math.max(...data.map((d) => d.v)) * 1.2 || 1
  const bw = (iw / data.length) * 0.62

  let g = ''
  for (let t = 0; t <= 3; t++) {
    const v = (yMax / 3) * t
    const yy = PT + ih - (v / yMax) * ih
    g += `<line x1="${PL}" y1="${yy.toFixed(1)}" x2="${(W - PR).toFixed(1)}" y2="${yy.toFixed(1)}" stroke="#eef2f7"/>`
    g += `<text x="${(PL - 5).toFixed(1)}" y="${(yy + 3).toFixed(1)}" font-size="7" fill="#94a3b8" text-anchor="end">${v.toFixed(0)}</text>`
  }
  data.forEach((d, i) => {
    const cx = PL + (i + 0.5) * (iw / data.length)
    const h = (d.v / yMax) * ih
    g += `<rect x="${(cx - bw / 2).toFixed(1)}" y="${(PT + ih - h).toFixed(1)}" width="${bw.toFixed(1)}" height="${h.toFixed(1)}" rx="2" fill="${d.color ?? '#3b82f6'}"/>`
    g += `<text x="${cx.toFixed(1)}" y="${(H - 14).toFixed(1)}" font-size="6.6" fill="#64748b" text-anchor="middle">${d.label}</text>`
    g += `<text x="${cx.toFixed(1)}" y="${(PT + ih - h - 3).toFixed(1)}" font-size="6.6" fill="#334155" text-anchor="middle">${d.v.toFixed(1)}</text>`
  })
  return `<svg viewBox="0 0 ${W} ${H}" style="width:100%;height:auto" xmlns="http://www.w3.org/2000/svg">${g}</svg>`
}

export function statusFor(value: number, warnAt: number, critAt: number): 'ok' | 'warn' | 'crit' {
  if (value >= critAt) return 'crit'
  if (value >= warnAt) return 'warn'
  return 'ok'
}

export const STATUS_LABEL: Record<'ok' | 'warn' | 'crit', string> = {
  ok: 'Pass',
  warn: 'Degraded',
  crit: 'Fail',
}