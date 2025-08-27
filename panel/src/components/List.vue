<script setup lang="ts">

const props = defineProps<{
  row?: boolean

}>()

</script>

<template>
  <div class="list element" :class="`${props.row?'list-row':'list-col'}`">
    <slot></slot>
  </div>
</template>

<style scoped>
.list {
  display: flex;
  border: 1px solid rgba(76,76,78, 0.1);
  border-radius: calc(0.7rem - 5px);
  //background-color: rgba(76,76,78, 0.025);
  margin: 5px;
}

.list-row {
  flex-direction: row;
}

.list-col {
  flex-direction: column;
}

</style>