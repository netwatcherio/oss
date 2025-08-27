<script setup lang="ts">

import core from "@/core";
import {reactive} from "vue";

const props = defineProps<{
  icon?: string
  title?: string
  code: boolean
  secondary?: string
  to?: string
}>()

const state = reactive({
  open: false,
  virgin: true,
})

const router = core.router()

function toggle() {
  state.open = !state.open
  state.virgin = false
}

function click(args: any) {
  toggle()
}

</script>

<template>
  <div class="element-link d-flex flex-column justify-content-center " @click="click">
    <div class="d-flex flex-row justify-content-between align-items-start pt-1">
      <div class="d-flex flex-row gap-2 align-items-center">
        <div v-if="props.icon" class="h-100 d-flex align-items-center text-primary">
          <i :class="props.icon" class="fa-1x fa-fw" style="font-size: 1.1rem"></i>
        </div>
        <div class="d-flex flex-column justify-content-center align-items-center gap-1 lh-1">
          <div class="label-o0 label-c4 label-w500">{{ props.title }}</div>
          <div class="label-o4 label-c5 ">{{ props.secondary }}</div>
        </div>
      </div>

      <div class="d-flex align-items-center justify-content-end gap-2 ">
        <div class="label-o4 label-c5" :class="`${props.code?'label-code':''}`">
          <slot></slot>
        </div>
        <div>
          <i :class="`${state.open?'animate-chevron':(!state.virgin?'animate-chevron-back':'')}`" class="fa-1x fa-fw fa-solid fa-chevron-left label-o5" ></i>

        </div>
      </div>
    </div>
    <div v-if=state.open class="d-flex justify-content-start animate-expand">
      <slot name="expanded" class=""></slot>
    </div>
  </div>

</template>

<style scoped>
.animate-chevron-back {
  animation: animate-chevron-close 200ms ease forwards;
}
.animate-chevron {
  animation: animate-chevron-open 200ms ease-in-out forwards;
}
.animate-expand {
  animation: animate-expand-in 200ms ease forwards;

}

@keyframes animate-chevron-close {
  0% { transform: rotate(-90deg) }
  100% { transform: rotate(0deg) }
}

@keyframes animate-chevron-open {
  0% { transform: rotate(0deg) }
  100% { transform: rotate(-90deg) }
}

@keyframes animate-expand-in {
  0% {
    clip-path: inset(0 0 100% 0);
    opacity: 0;
    filter: blur(2px);
  }
  50% {
    filter: blur(0);
    opacity: 0.5;
  }
  100% {
    opacity: 1;
    clip-path: inset(0);
  }
}


.element-link:hover {
  background-color: rgba(142, 142, 145, 0.05);
//border-radius: 0.375rem;
}

.element-link {
  padding: 0.5rem 0.8rem;
  cursor: pointer;
  transition: background-color 100ms;
  flex-grow: 1;
//box-shadow: 0 0 0px 1px red;
}

.element-link:not(:last-of-type) {
  border-bottom: 1px solid rgba(0, 0, 0, 0.1);
}
</style>