<script lang="ts" setup>
import { reactive } from "vue";
import { useRouter } from "vue-router";
import Loader from "@/components/Loader.vue";
import AuthLayout from "@/components/AuthLayout.vue";
import { AuthService } from "@/services/apiService";

const router = useRouter();

const state = reactive({
  email: "",
  waiting: false,
  error: false,
  errorMessage: "",
  success: false,
});

function begin() {
  state.waiting = true;
  state.error = false;
  state.success = false;
}

function done() {
  state.waiting = false;
}

async function submit(e: Event) {
  e.preventDefault();
  
  if (!state.email) {
    state.error = true;
    state.errorMessage = "Please enter your email address.";
    return;
  }
  
  begin();
  
  try {
    await AuthService.requestPasswordReset(state.email);
    done();
    state.success = true;
  } catch (err: any) {
    done();
    state.error = true;
    
    const serverError = err?.response?.data?.error ?? err?.message ?? "";
    
    if (serverError === "user not found" || serverError === "User not found") {
      // Don't reveal if user exists - show generic message
      state.success = true;
    } else {
      state.errorMessage = "Unable to process request. Please try again later.";
    }
  }
}
</script>

<template>
  <AuthLayout>
    <div class="card">
      <div class="card-body">
        <div class="d-flex align-items-end justify-content-between gap-2 mb-3">
          <h2 class="auth-title mb-0">Reset Password</h2>
          <span class="auth-subtext">
            Remember your password?
            <router-link to="/auth/login">Login</router-link>
          </span>
        </div>

        <!-- Success State -->
        <div v-if="state.success" class="success-content">
          <div class="success-icon">
            <i class="bi bi-envelope-check"></i>
          </div>
          <h4 class="mt-3">Check Your Email</h4>
          <p class="text-muted mb-4">
            If an account exists for <strong>{{ state.email }}</strong>, 
            we've sent a password reset link. Please check your inbox and spam folder.
          </p>
          <router-link to="/auth/login" class="btn btn-primary">
            Back to Login
          </router-link>
        </div>

        <!-- Form State -->
        <template v-else>
          <p class="text-muted small mb-3">
            Enter your email address and we'll send you a link to reset your password.
          </p>

          <form class="auth-form" @submit="submit">
            <div class="form-floating mb-3">
              <input
                id="reset-email"
                v-model="state.email"
                class="form-control"
                name="email"
                placeholder="name@example.com"
                required
                type="email"
                autocomplete="email"
              />
              <label for="reset-email">Email</label>
            </div>

            <div class="d-flex align-items-center justify-content-between">
              <router-link to="/auth/login" class="auth-link">
                <i class="bi bi-arrow-left me-1"></i>
                Back to Login
              </router-link>

              <div class="d-flex align-items-center gap-3">
                <Loader v-if="state.waiting" inverse />
                <button
                  type="submit"
                  class="btn btn-primary btn-lg px-4"
                  :disabled="state.waiting"
                >
                  Send Reset Link
                </button>
              </div>
            </div>
          </form>

          <!-- Error message -->
          <div v-if="state.error && !state.waiting" class="error-message">
            <i class="bi bi-exclamation-triangle me-2"></i>
            {{ state.errorMessage }}
          </div>
        </template>
      </div>
    </div>
  </AuthLayout>
</template>

<style scoped>
.auth-title {
  font-size: 1.5rem;
  font-weight: 600;
}

.auth-subtext {
  font-size: 0.875rem;
  color: var(--bs-secondary-color);
}

.auth-subtext a {
  font-weight: 500;
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

.success-content {
  text-align: center;
  padding: 1rem 0;
}

.success-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 4rem;
  height: 4rem;
  border-radius: 50%;
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
  font-size: 2rem;
}
</style>