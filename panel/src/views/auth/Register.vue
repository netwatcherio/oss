<script lang="ts" setup>
import { useRouter } from "vue-router";
import { reactive, onMounted } from "vue";
import Loader from "@/components/Loader.vue";
import AuthLayout from "@/components/AuthLayout.vue";

// NEW: use the new auth service
import { AuthService } from "@/services/apiService";

const router = useRouter();

type User = {
  name?: string;
  email: string;
  password: string;
};

const state = reactive({
  user: {} as User,
  confirmPassword: "",
  errorMessage: "",
  waiting: false,
  began: 0,
  error: false,
  registrationDisabled: false, // Will be set on mount if registration is disabled
});

// Check if registration is enabled on mount
onMounted(async () => {
  try {
    const config = await AuthService.getConfig();
    if (!config.registration_enabled) {
      state.registrationDisabled = true;
      state.error = true;
      state.errorMessage = "Registration is currently disabled. Please contact your administrator.";
    }
  } catch {
    // If config fetch fails, allow registration attempt (server will still block if disabled)
  }
});

function begin() {
  state.waiting = true;
  state.began = Date.now().valueOf();
}

function done() {
  if (!state.waiting) return;
  const delta = Date.now().valueOf() - state.began;
  const minTimeout = 250;
  setTimeout(() => {
    state.waiting = false;
    state.began = 0;
  }, Math.max(minTimeout - delta, 0));
}

function onRegister(_: unknown) {
  done();
  // Optional: show a toast/snackbar "Registered! Please log in."
  router.push("/auth/login");
}

function onFailure(error: any) {
  done();
  state.error = true;

  // Try common shapes: { error: "..." } or plain string
  const server =
      error?.response?.data?.error ??
      error?.response?.data ??
      error?.message ??
      "";

  switch (server) {
    case "registration is disabled":
      state.errorMessage = "Registration is currently disabled. Please contact your administrator.";
      state.registrationDisabled = true;
      break;
    case "user exists":
    case "User already exists":
      state.errorMessage = "The email provided has already been registered for an account.";
      break;
    case "invalid":
    case "Invalid request":
    case "validation_error":
      state.errorMessage = "Invalid request. Please check your information.";
      break;
    default:
      state.errorMessage = "An error occurred. Please try again later.";
      break;
  }
}

async function submit(e: MouseEvent) {
  e.preventDefault();

  // Block submission if registration is disabled
  if (state.registrationDisabled) {
    state.error = true;
    state.errorMessage = "Registration is currently disabled. Please contact your administrator.";
    return;
  }

  // client-side confirm password check
  if (!state.user.password || state.user.password !== state.confirmPassword) {
    state.error = true;
    state.errorMessage = "Passwords do not match.";
    return;
  }

  begin();
  try {
    const body = {
      email: state.user.email,
      password: state.user.password,
      name: state.user.name,
    };
    await AuthService.register(body);
    onRegister(null);
  } catch (err) {
    onFailure(err);
  }
}
</script>

<template>
  <AuthLayout>
    <div class="card">
      <div class="card-body">
        <div class="d-flex align-items-end justify-content-between gap-2 mb-3">
          <h2 class="auth-title mb-0">Register</h2>
          <span class="auth-subtext">
            Have an account?
            <router-link id="to-login" to="/auth/login">Login</router-link>
          </span>
        </div>

        <form class="auth-form" @submit.prevent="submit">
          <div class="form-floating mb-3">
            <input
              id="nw-name"
              v-model="state.user.name"
              class="form-control"
              name="name"
              placeholder="John Doe"
              required
              type="text"
              autocomplete="name"
            />
            <label for="nw-name">Name</label>
          </div>

          <div class="form-floating mb-3">
            <input
              id="nw-email"
              v-model="state.user.email"
              class="form-control"
              name="email"
              placeholder="john@example.com"
              required
              type="email"
              autocomplete="email"
            />
            <label for="nw-email">Email</label>
          </div>

          <div class="form-floating mb-3">
            <input
              id="nw-password"
              v-model="state.user.password"
              class="form-control"
              name="password"
              placeholder="*****"
              required
              type="password"
              autocomplete="new-password"
            />
            <label for="nw-password">Password</label>
          </div>

          <div class="form-floating mb-3">
            <input
              id="nw-confirm-password"
              v-model="state.confirmPassword"
              class="form-control"
              :class="{ 'is-invalid': state.confirmPassword && state.confirmPassword !== state.user.password }"
              name="confirm_password"
              placeholder="*****"
              required
              type="password"
              autocomplete="new-password"
            />
            <label for="nw-confirm-password">Confirm Password</label>
            <div class="invalid-feedback" v-if="state.confirmPassword && state.confirmPassword !== state.user.password">
              Passwords do not match
            </div>
          </div>

          <div class="d-flex align-items-center justify-content-end gap-3">
            <Loader v-if="state.waiting" inverse />
            <button
              type="submit"
              class="btn btn-primary btn-lg px-4"
              :disabled="state.waiting || state.registrationDisabled"
            >
              Register
            </button>
          </div>
        </form>

        <!-- Error message -->
        <div v-if="state.error && !state.waiting" class="error-message">
          <i class="bi bi-exclamation-triangle me-2"></i>
          {{ state.errorMessage }}
        </div>
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
</style>