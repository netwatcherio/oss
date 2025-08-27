<script setup lang="ts">

const props = defineProps<{
  title?: string,

}>()

</script>

<template>
  <div class="card">
    <div class="card-body d-flex flex-column gap-1">
      <div class="d-flex align-items-end justify-content-between gap-1">
        <div class="d-flex gap-2">
          <div class="label-title">Login</div>
        </div>
        <slot name="alternate">
          Alt
        </slot>


      </div>
      <slot name="body">
        Body
      </slot>
    </div>
  </div>
</template>

<style scoped>

</style>