<script setup lang="ts">
import {v4 as uuidv4} from "uuid";
import {onMounted, reactive} from "vue";

import {mkSimplexNoise} from "@/composables/noise";
import {FillChart} from "@/composables/fillChart";

const props = defineProps<{
  data?: number[]
  labels?: string[]
  values?: string[]
}>()

const state = reactive({
  chart: {} as FillChart,
  uuid: uuidv4(),
  data: [] as number[],
  ready: false
})

onMounted(() => {

  state.data = props.data || [];
  state.chart = new FillChart(getUuid(), state.data as number[])
  state.chart.draw()
  state.ready = true;
  // draw()
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
  return `fillchart-${state.uuid}`
}


</script>

<template>
  <div class="d-flex align-items-center gap-2">
    <div v-if="state.ready"  class="d-flex flex-column label-o3 label-c6">
      <div v-for="(label, index) in props.labels" :style="`color: rgba(${state.chart.pickColor(index)},1)`" class="d-flex flex-row gap-2 justify-content-between">
        <div class="d-flex justify-content-end label-o4" style="width: 5rem">{{label}}</div>
        <div v-if=!props.values style="width: 4.5rem" class="d-flex align-items-end justify-content-end">{{(Math.round(state.data[index] * 1000)/10).toFixed(2)}}%</div>
        <div v-else style="width: 4.5rem" class="d-flex align-items-end justify-content-end">{{props.values[index]}}</div>
      </div>

    </div>
    <div>
      <canvas :id="getUuid()" class="chart-canvas"></canvas>
    </div>
  </div>

</template>

<style lang="scss" scoped>
.chart-canvas {
  height: 2rem;
  width: 8rem;
  border: 1px solid rgba(0,0,0,0.1);
  border-radius: 4px;
}
</style>