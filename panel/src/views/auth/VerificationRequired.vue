<script lang="ts" setup>
import { reactive, computed, onUnmounted } from "vue";
import AuthLayout from "@/components/AuthLayout.vue";
import Loader from "@/components/Loader.vue";
import { AuthService } from "@/services/apiService";
import { getSession, clearSession } from "@/session";
import core from "@/core";

const router = core.router();

const state = reactive({
  sending: false,
  sent: false,
  error: "",
  cooldown: 0,
});

// Get user email from session
const userEmail = computed(() => {
  const session = getSession();
  return session?.user?.email || "your email";
});

let cooldownTimer: ReturnType<typeof setInterval> | null = null;

function startCooldown(seconds: number) {
  state.cooldown = seconds;
  if (cooldownTimer) clearInterval(cooldownTimer);
  cooldownTimer = setInterval(() => {
    state.cooldown--;
    if (state.cooldown <= 0) {
      if (cooldownTimer) clearInterval(cooldownTimer);
      cooldownTimer = null;
    }
  }, 1000);
}

onUnmounted(() => {
  if (cooldownTimer) clearInterval(cooldownTimer);
});

async function resend() {
  if (state.sending || state.cooldown > 0) return;
  state.sending = true;
  state.error = "";
  state.sent = false;

  try {
    await AuthService.resendVerification();
    state.sent = true;
    startCooldown(60);
  } catch (err: any) {
    const serverError = err?.response?.data?.error ?? "";
    if (serverError.includes("already verified")) {
      // User got verified meanwhile — redirect to app
      router.push("/");
      return;
    }
    state.error = serverError || "Failed to send verification email. Please try again.";
  } finally {
    state.sending = false;
  }
}

function logout() {
  clearSession();
  router.push("/auth/login");
}
</script>

<template>
  <AuthLayout>
    <div class="card">
      <div class="card-body">
        <div class="status-content">
          <div class="status-icon verify-icon">
            <i class="bi bi-envelope-exclamation"></i>
          </div>

          <h2 class="auth-title mt-3 mb-2">Verify Your Email</h2>

          <p class="text-muted mb-1">
            To continue using NetWatcher, please verify your email address.
          </p>
          <p class="text-muted mb-4">
            We sent a verification link to
            <strong>{{ userEmail }}</strong>.
            Check your inbox (and spam folder) for the email.
          </p>

          <!-- Resend button -->
          <button
            class="btn btn-primary btn-lg px-4 mb-3"
            :disabled="state.sending || state.cooldown > 0"
            @click="resend"
          >
            <Loader v-if="state.sending" inverse />
            <template v-else-if="state.cooldown > 0">
              Resend in {{ state.cooldown }}s
            </template>
            <template v-else>
              <i class="bi bi-envelope me-2"></i>
              Resend Verification Email
            </template>
          </button>

          <!-- Success feedback -->
          <div v-if="state.sent && !state.error" class="success-message mb-3">
            <i class="bi bi-check-circle me-2"></i>
            Verification email sent! Please check your inbox.
          </div>

          <!-- Error feedback -->
          <div v-if="state.error" class="error-message mb-3">
            <i class="bi bi-exclamation-triangle me-2"></i>
            {{ state.error }}
          </div>

          <div class="divider"></div>

          <p class="text-muted small mb-2">
            Already verified?
            <a href="/" class="fw-medium">Go to Dashboard</a>
          </p>

          <a href="#" class="auth-link" @click.prevent="logout">
            <i class="bi bi-box-arrow-left me-1"></i>
            Sign out
          </a>
        </div>
      </div>
    </div>
  </AuthLayout>
</template>

<style scoped>
.auth-title {
  font-size: 1.5rem;
  font-weight: 600;
  text-align: center;
}

.status-content {
  text-align: center;
  padding: 1.5rem 0;
}

.status-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 4.5rem;
  height: 4.5rem;
  border-radius: 50%;
  font-size: 2.25rem;
}

.verify-icon {
  background: rgba(234, 179, 8, 0.15);
  color: #eab308;
}

.divider {
  border-top: 1px solid var(--bs-border-color);
  margin: 1rem 0;
}

.auth-link {
  font-size: 0.875rem;
  font-weight: 500;
  text-decoration: none;
  opacity: 0.8;
  transition: opacity 0.2s;
}

.auth-link:hover {
  opacity: 1;
}
</style>
