<script lang="ts" setup>
import { reactive, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import Loader from "@/components/Loader.vue";
import AuthLayout from "@/components/AuthLayout.vue";
import { AuthService } from "@/services/apiService";

const route = useRoute();
const router = useRouter();

const state = reactive({
  token: "" as string,
  newPassword: "",
  confirmPassword: "",
  waiting: false,
  error: false,
  errorMessage: "",
  success: false,
  validating: true,
});

onMounted(() => {
  const token = route.params.token as string;
  if (!token) {
    state.error = true;
    state.errorMessage = "Invalid reset link. No token provided.";
    state.validating = false;
    return;
  }
  state.token = token;
  state.validating = false;
});

function begin() {
  state.waiting = true;
  state.error = false;
}

function done() {
  state.waiting = false;
}

async function submit(e: Event) {
  e.preventDefault();

  if (!state.newPassword || !state.confirmPassword) {
    state.error = true;
    state.errorMessage = "Please fill in both password fields.";
    return;
  }

  if (state.newPassword !== state.confirmPassword) {
    state.error = true;
    state.errorMessage = "Passwords do not match.";
    return;
  }

  if (state.newPassword.length < 6) {
    state.error = true;
    state.errorMessage = "Password must be at least 6 characters long.";
    return;
  }

  begin();

  try {
    await AuthService.completePasswordReset(state.token, state.newPassword);
    done();
    state.success = true;
  } catch (err: any) {
    done();
    state.error = true;

    const serverError = err?.response?.data?.error ?? "";
    if (serverError.includes("expired")) {
      state.errorMessage = "This reset link has expired. Please request a new one.";
    } else if (serverError.includes("not found")) {
      state.errorMessage = "This reset link is invalid or has already been used.";
    } else {
      state.errorMessage = "Unable to reset password. Please try again or request a new link.";
    }
  }
}
</script>

<template>
  <AuthLayout>
    <div class="card">
      <div class="card-body">
        <div class="d-flex align-items-end justify-content-between gap-2 mb-3">
          <h2 class="auth-title mb-0">Set New Password</h2>
          <span class="auth-subtext">
            <router-link to="/auth/login">Back to Login</router-link>
          </span>
        </div>

        <!-- Loading -->
        <div v-if="state.validating" class="text-center py-4">
          <Loader inverse />
          <p class="text-muted mt-2">Validating reset link...</p>
        </div>

        <!-- Success State -->
        <div v-else-if="state.success" class="success-content">
          <div class="success-icon">
            <i class="bi bi-check-circle"></i>
          </div>
          <h4 class="mt-3">Password Reset Successfully</h4>
          <p class="text-muted mb-4">
            Your password has been updated. You can now log in with your new password.
          </p>
          <router-link to="/auth/login" class="btn btn-primary">
            Go to Login
          </router-link>
        </div>

        <!-- Form State -->
        <template v-else>
          <p class="text-muted small mb-3">
            Enter your new password below.
          </p>

          <form class="auth-form" @submit="submit">
            <div class="form-floating mb-3">
              <input
                id="new-password"
                v-model="state.newPassword"
                class="form-control"
                name="new-password"
                placeholder="New password"
                required
                type="password"
                autocomplete="new-password"
              />
              <label for="new-password">New Password</label>
            </div>

            <div class="form-floating mb-3">
              <input
                id="confirm-password"
                v-model="state.confirmPassword"
                class="form-control"
                name="confirm-password"
                placeholder="Confirm password"
                required
                type="password"
                autocomplete="new-password"
              />
              <label for="confirm-password">Confirm Password</label>
            </div>

            <div class="d-flex align-items-center justify-content-between">
              <router-link to="/auth/reset" class="auth-link">
                <i class="bi bi-arrow-left me-1"></i>
                Request new link
              </router-link>

              <div class="d-flex align-items-center gap-3">
                <Loader v-if="state.waiting" inverse />
                <button
                  type="submit"
                  class="btn btn-primary btn-lg px-4"
                  :disabled="state.waiting"
                >
                  Reset Password
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
