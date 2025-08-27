<script setup lang="ts">

const props = defineProps<{
  icon?: string
  title?: string
  subtitle?: string
}>()

</script>

<template>
  <div class="element card">
    <div class="px-2 py-2 d-flex justify-content-between align-items-center" v-if="props.title">
      <div class="d-flex align-items-start justify-content-center gap-2">
        <div>
          <i v-if="props.icon" :class="props.icon"></i>
        </div>
        <div class="d-flex flex-column justify-content-center">
          <div class="label-c4 label-o2 label-w500">{{ props.title }}</div>
          <div class="label-c6 label-o4 label-w400">{{ props.subtitle }}</div>

        </div>
      </div>
      <slot name="secondary"></slot>
    </div>
    <slot></slot>
  </div>
</template>

<style scoped>
.element {
  padding: 0 0;
}
</style>