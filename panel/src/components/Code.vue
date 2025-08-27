<script setup lang="ts">
import { onUnmounted, reactive, watchEffect } from 'vue';

const props = defineProps<{
  title?: string;
  visible?: boolean;
  code: string;
}>();

const state = reactive({
  copied: false,
  displayText: '',
  scrambling: false,
  scrambleRate: 0,
  visible: false,
  showTimer: 0,
  scrambleAnimation: 0,
  reset: 0,
});

function copyText() {
  state.copied = true;
  navigator.clipboard.writeText(props.code);
  setTimeout(() => {
    state.copied = false;
  }, 1500);
}

onUnmounted(() => {
  clearTimeout(state.showTimer);
  clearInterval(state.scrambleAnimation);
});

watchEffect(() => {
  state.displayText = displayTest();
  return state.visible;
});

function resetShowTimer(aaa: any) {
  state.visible = true;
  clearTimeout(state.showTimer);
  clearInterval(state.scrambleAnimation);
  state.showTimer = setTimeout(() => {
    state.visible = false;
  }, 1000);
}

function displayTest(): string {
  if (props.visible || state.visible) {
    return props.code;
  }

  return '*'.repeat(props.code.length);
}
</script>

<template>
  <div
      class="code-copy label-code d-flex flex-column flex-sm-row gap-1 align-items-center code-field"
      @click="copyText"
  >
    <div
        v-if="props.title"
        class="label-c5 text-primary px-1 text-nowrap"
    >
      {{ props.title }}
    </div>
    <div class="d-flex flex-grow-1 gap-1 justify-content-between w-100">
      <div class="label-o4 flex-grow-1 text-truncate">
        {{ state.displayText }}
      </div>
      <div class="d-flex gap-1">
        <span class="copy-box">
          <i v-if="!state.copied" class="bi bi-clipboard"></i>
          <i v-else class="bi bi-clipboard-check copied"></i>
        </span>
      </div>
    </div>
  </div>
</template>

<style>
.copy-box {
  padding: 0px 6px;
  background-color: rgba(141, 141, 143, 0.2);
  border-radius: 4px;
}

.code-field {
  background-color: rgba(174, 174, 175, 0.2);
  border-radius: 8px;
  padding: 0.25rem 0.25rem;
  width: 100%;
}

@keyframes isCopied {
  0% {
    opacity: 0.75;
  }
  100% {
    opacity: 1;
  }
}

i.copied {
  width: inherit;
  height: inherit;
  animation: isCopied 250ms forwards ease;
}

/* Responsive Adjustments */
@media (max-width: 576px) {
  .code-copy {
    flex-direction: column;
    align-items: stretch;
  }
  .code-field {
    padding: 0.5rem;
  }
  .label-c5 {
    margin-bottom: 0.5rem;
  }
}
</style>
