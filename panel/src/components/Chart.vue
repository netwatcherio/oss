<script setup lang="ts">
import {v4 as uuidv4} from "uuid";
import {onMounted, reactive} from "vue";
import {Chart, ChartStyle} from "@/composables/chart";
import type {SimplexNoise} from "@/composables/noise";
import {mkSimplexNoise} from "@/composables/noise";

const props = defineProps<{
  overlay?: string
}>()

const state = reactive({
  chart: {} as Chart,
  uuid: uuidv4(),
  data: [] as number[]
})

onMounted(() => {
  let n = mkSimplexNoise(Math.random)
  for (let i = 0; i < 24; i++) {
  state.data.push(n.noise2D(i,0))

  }
  state.chart = new Chart(getUuid(), ChartStyle.TrendLine, state.data as number[])
  state.chart.draw()
  draw()
})

function draw() {
  // requestAnimationFrame(draw)
  let n = mkSimplexNoise(Math.random)
  let speed = 2;

  for (let i = 0; i < speed; i++) {
    state.data.push(n.noise2D(i,0))
  }

  if(state.data.length > 24) {
    state.data = state.data.slice(speed)
  }
  state.chart.data = state.data
  state.chart.draw()
}

function getUuid(): string {
  return `chart-${state.uuid}`
}


</script>

<template>
<div class="d-flex align-items-center justify-content-center">
  <canvas :id="getUuid()" class="chart-canvas"></canvas>
</div>
</template>

<style lang="scss" scoped>
.chart-canvas {
  height: 2rem;
  width: 8rem;
  border: 1px solid rgba(0,0,0,0.1);
  border-radius: 8px;
}
</style>