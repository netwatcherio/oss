<script setup lang="ts">

import Chart from "@/components/Chart.vue"

let props = defineProps<{
  title: string,
  subtext: string,
  desc: string
}>()
</script>

<template>
  <div style="height: 12rem" class="w-100">
  <div class="card element h-100 px-3 py-3">

  <div class="d-flex flex-column justify-content-center">
    <div class="d-flex justify-content-between">
      <div class="d-flex flex-column align-items-start justify-content-center">
        <div class="fw-bold lh-1">Title</div>
        <small class="text-muted lh-1">Subtitle</small>
      </div>
      <div class="d-flex align-items-center justify-content-end">
      <Chart style="height: 2rem"></Chart>
      <div class="d-flex flex-column align-items-end justify-content-center">
        <div class="fw-bold lh-1">100Mbps</div>

      </div>
      </div>
    </div>
    <hr>
    <div class="d-flex justify-content-between">
      <div class="d-flex flex-column align-items-start justify-content-center">
        <div class="fw-bold lh-1">Title</div>
        <small class="text-muted lh-1">Subtitle</small>
      </div>
      <div class="d-flex flex-column align-items-end justify-content-center">
        <div class="fw-bold lh-1">Title</div>
        <small class="text-muted lh-1">Subtitle</small>
      </div>
    </div>
    <hr>
    <div class="d-flex justify-content-between">
      <div class="d-flex flex-column align-items-start justify-content-center">
        <div class="fw-bold lh-1">Title</div>
        <small class="text-muted lh-1">Subtitle</small>
      </div>
      <div class="d-flex flex-column align-items-start justify-content-center">
        <div class="fw-bold lh-1">Title</div>
        <small class="text-muted lh-1">Subtitle</small>
      </div>
    </div>
  </div>
  </div>
  </div>
</template>

<style lang="scss" scoped>
.speed-test {

}
</style>