
<script setup lang="ts">
const props = defineProps<{
  inverse?: boolean
  large?: boolean
}>()
</script>


<template>
  <span class="loader" :class="`${props.inverse?'loader-dark':''} ${props.large?'loader-large':''}`"></span>
</template>


<style lang="scss" scoped>
$primary: #206bc4;

.loader {
  width: 0.8rem;
  height: 0.8rem;
  border: 2px solid #FFF;
  border-bottom-color: transparent;
  border-radius: 50%;
  display: inline-block;
  box-sizing: border-box;
  animation: rotation 1s linear infinite;
}

.loader-dark {
  width: 0.8rem;
  height: 0.8rem;
  border: 2px solid $primary !important;
  border-bottom-color: rgba(0,0,0,0.1) !important;
//border-top-color: rgba(0,0,0,0.1) !important;
  border-radius: 50%;
  display: inline-block;
  box-sizing: border-box;
  animation: rotation 1s linear infinite
}

.loader.loader-large {

  width: 1.25rem !important;
  height: 1.25rem !important;
  border-width: 3px !important;
}

</style>