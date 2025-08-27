<script lang="ts" setup>

import core from "@/core";

const props = defineProps<{
  title: string,
  subtitle?: string,
  history?: any[],
}>()

const router = core.router()

</script>

<template>
  <div class="d-flex mx-1 mt-3 align-items-end justify-content-between mb-2">
    <div class="align-self-center">
      <nav aria-label="breadcrumb">
        <ol class="breadcrumb mb-1 d-flex align-items-center px-1">
          <li class="breadcrumb-item ">
            <router-link to="/" class="text-muted"><i class="bi bi-house"></i></router-link>
          </li>
          <li v-if="props.history" v-for="page in props.history" class="breadcrumb-item active" aria-current="page">
            <router-link :to="page.link" class="">{{ page.title }}</router-link>
          </li>
          <li class="breadcrumb-item active" aria-current="page">
            {{ props.title }}
          </li>
        </ol>
      </nav>
      <div class="d-flex justify-content-between fade-in">
        <div class="d-flex align-items-center gap-2">
          <div v-if="props.history" class="">
            <router-link :to="props.history[props.history.length-1].link" class="btn btn-primary"><i class="fa-solid fa-chevron-left"></i></router-link>
          </div>
          <div class="fw-bold lh-1 h2 mb-0" >{{ props.title }}</div>


        </div>

      </div>
        <div v-if="props.subtitle" class="text-muted">{{ props.subtitle }}</div>

    </div>
    <div>
      <slot></slot>
    </div>
  </div>
</template>

<style lang="scss">
.fade-in {
  animation: fadeIn 200ms forwards ease;
}

@keyframes fadeIn {
  0% {
    opacity: 0.5;
    transform: translate(0.25rem, 0) scale(0.998);
  }
  100% {
    opacity: 1;
    transform: translate(0, 0) scale(1);
  }
}
</style>