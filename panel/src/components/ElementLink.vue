<script setup lang="ts">

import core from "@/core";

const props = defineProps<{
  icon?: string
  title?: string
  secondary?: string
  to?: string
}>()

const router = core.router()

function click(args: any) {
  if(props.to) {
    router.push(props.to)
  }
}

</script>

<template>
  <div class="element-link d-flex flex-row justify-content-between align-items-center" @click="click">
    <div class="d-flex flex-row gap-2 align-items-center">
      <div v-if="props.icon" class="h-100 d-flex align-items-center text-primary">
        <i :class="props.icon" class="fa-1x fa-fw" style="font-size: 1.1rem"></i>
      </div>
      <div class="d-flex flex-column justify-content-center align-items-start gap-1">
        <div class="label-o0 label-c4 label-w600">{{ props.title }}</div>
        <div class="label-o4 label-c5 ">{{ props.secondary }}</div>
      </div>
    </div>

    <div class="d-flex align-items-center justify-content-end gap-2">
      <div class="label-o4 label-c4">
        <slot></slot>
      </div>
      <i class="fa-solid fa-chevron-right label-o6"></i>
    </div>
  </div>
</template>

<style scoped>
.element-link:hover {
  background-color: rgba(142,142,145,0.05);
  //border-radius: 0.375rem;
}

.element-link {
  padding: 0.5rem 0.8rem;
  cursor: pointer;
  transition: background-color 100ms;
  //box-shadow: 0 0 0px 1px red;
}
.element-link:not(:last-of-type) {
border-bottom: 1px solid rgba(0,0,0,0.1);
}
</style>