<template>
  <div class="recommendations-panel">
    <!-- Tests Section -->
    <div class="reco-section">
      <div class="section-header">
        <i class="bi bi-terminal"></i>
        <span>Recommended Tests</span>
        <span class="section-count">{{ tests.length }}</span>
      </div>
      <div class="section-content">
        <div class="test-list">
          <div 
            v-for="(test, idx) in tests" 
            :key="idx"
            class="test-card"
          >
            <div class="test-header">
              <span class="test-number">{{ idx + 1 }}</span>
              <h4 class="test-title">{{ test.test }}</h4>
            </div>
            <div class="test-command">
              <div class="command-label">
                <i class="bi bi-code-square"></i>
                Command
              </div>
              <div class="command-box">
                <code>{{ test.command_example }}</code>
                <button class="copy-btn" @click="copyCommand(test.command_example)" :title="'Copy command'">
                  <i class="bi bi-clipboard"></i>
                </button>
              </div>
            </div>
            <div class="test-why">
              <div class="why-label">
                <i class="bi bi-info-circle"></i>
                Why
              </div>
              <p>{{ test.why }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Questions Section -->
    <div class="reco-section">
      <div class="section-header">
        <i class="bi bi-chat-left-quote"></i>
        <span>Questions for Upstream</span>
        <span class="section-count">{{ questions.length }}</span>
      </div>
      <div class="section-content">
        <div class="question-list">
          <div 
            v-for="(q, idx) in questions" 
            :key="idx"
            class="question-card"
          >
            <div class="question-header">
              <span class="question-number">Q{{ idx + 1 }}</span>
              <div class="related-flags" v-if="q.related_flags.length">
                <span 
                  v-for="flag in q.related_flags" 
                  :key="flag"
                  class="flag-tag"
                >
                  {{ formatFlag(flag) }}
                </span>
              </div>
            </div>
            <p class="question-text">{{ q.question }}</p>
            <div class="question-why">
              <i class="bi bi-arrow-return-right"></i>
              <span>{{ q.why }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Copy Notification -->
    <Transition name="fade">
      <div v-if="showCopied" class="copy-notification">
        <i class="bi bi-check-circle-fill"></i>
        Command copied to clipboard
      </div>
    </Transition>
  </div>
</template>

<script lang="ts" setup>
import { ref } from 'vue';
import type { UpstreamQuestion, RecommendedTest } from './types';

const props = defineProps<{
  questions: UpstreamQuestion[];
  tests: RecommendedTest[];
}>();

const showCopied = ref(false);

async function copyCommand(command: string) {
  try {
    await navigator.clipboard.writeText(command);
    showCopied.value = true;
    setTimeout(() => {
      showCopied.value = false;
    }, 2000);
  } catch (err) {
    console.error('Failed to copy:', err);
  }
}

function formatFlag(flag: string): string {
  return flag.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
}
</script>

<style scoped>
.recommendations-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  position: relative;
}

/* Section */
.reco-section {
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 12px;
  overflow: hidden;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem 1.25rem;
  background: var(--bs-secondary-bg);
  font-weight: 600;
  font-size: 0.95rem;
  color: var(--bs-body-color);
}

.section-count {
  margin-left: auto;
  padding: 0.2rem 0.6rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  background: var(--bs-primary);
  color: white;
}

.section-content {
  padding: 1.25rem;
}

/* Test Cards */
.test-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.test-card {
  background: var(--bs-body-bg);
  border-radius: 10px;
  padding: 1.25rem;
  border: 1px solid var(--bs-border-color);
}

.test-header {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.test-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: var(--bs-primary);
  color: white;
  border-radius: 6px;
  font-weight: 700;
  font-size: 0.85rem;
  flex-shrink: 0;
}

.test-title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--bs-body-color);
  line-height: 1.4;
}

.test-command {
  margin-bottom: 1rem;
}

.command-label, .why-label {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--bs-secondary-color);
  letter-spacing: 0.5px;
  margin-bottom: 0.5rem;
}

.command-box {
  display: flex;
  align-items: center;
  background: var(--bs-tertiary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  overflow: hidden;
}

.command-box code {
  flex: 1;
  padding: 0.75rem 1rem;
  font-family: 'JetBrains Mono', var(--bs-font-monospace);
  font-size: 0.8rem;
  color: var(--bs-success);
  background: transparent;
  overflow-x: auto;
  white-space: nowrap;
}

.copy-btn {
  padding: 0.75rem 1rem;
  background: transparent;
  border: none;
  border-left: 1px solid var(--bs-border-color);
  color: var(--bs-secondary-color);
  cursor: pointer;
  transition: all 0.15s;
}

.copy-btn:hover {
  background: var(--bs-secondary-bg);
  color: var(--bs-primary);
}

.test-why p {
  margin: 0;
  font-size: 0.85rem;
  color: var(--bs-body-color);
  line-height: 1.5;
}

/* Question Cards */
.question-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.question-card {
  background: var(--bs-body-bg);
  border-radius: 10px;
  padding: 1.25rem;
  border: 1px solid var(--bs-border-color);
  border-left: 3px solid var(--bs-info);
}

.question-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
  flex-wrap: wrap;
}

.question-number {
  font-size: 0.75rem;
  font-weight: 700;
  padding: 0.2rem 0.5rem;
  background: var(--bs-info);
  color: white;
  border-radius: 4px;
}

.related-flags {
  display: flex;
  gap: 0.35rem;
  flex-wrap: wrap;
}

.flag-tag {
  font-size: 0.65rem;
  padding: 0.15rem 0.5rem;
  background: rgba(var(--bs-warning-rgb), 0.15);
  color: var(--bs-warning);
  border-radius: 3px;
  font-weight: 500;
}

.question-text {
  margin: 0 0 0.75rem 0;
  font-size: 0.95rem;
  font-weight: 500;
  color: var(--bs-body-color);
  line-height: 1.5;
}

.question-why {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  font-size: 0.8rem;
  color: var(--bs-secondary-color);
  padding: 0.75rem;
  background: var(--bs-tertiary-bg);
  border-radius: 8px;
  line-height: 1.5;
}

.question-why i {
  flex-shrink: 0;
  margin-top: 0.15rem;
}

/* Copy Notification */
.copy-notification {
  position: fixed;
  bottom: 2rem;
  left: 50%;
  transform: translateX(-50%);
  padding: 0.75rem 1.5rem;
  background: var(--bs-success);
  color: white;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9rem;
  font-weight: 500;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  z-index: 1000;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(10px);
}
</style>
