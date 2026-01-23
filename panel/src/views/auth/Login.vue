<script lang="ts" setup>
import { reactive, onMounted } from "vue";
import core from "@/core";
import Loader from "@/components/Loader.vue";
import AuthLayout from "@/components/AuthLayout.vue";

// NEW imports: use AuthService + token helpers from the new stack
import { AuthService } from "@/services/apiService";
import { setSession, getSession } from "@/session";

const state = reactive({
  user: {} as User,
  began: 0 as number,
  waiting: false,
  error: false,
  registrationEnabled: true, // Default to true until we know
});

const router = core.router();

// Check if registration is enabled
onMounted(async () => {
  try {
    const config = await AuthService.getConfig();
    state.registrationEnabled = config.registration_enabled;
  } catch {
    // If config fetch fails, default to showing registration
    state.registrationEnabled = true;
  }
});

// Minimal user type for this form
interface User {
  email: string;
  password: string;
}

// Auth response shape from new service
interface LoginResponse {
  token: string;
  data?: any; // user object; keep loose to avoid tight coupling
}

let legacySession = core.session?.();

function onLogin(payload: LoginResponse) {
  done();

  // Persist via new session helper
  setSession({ token: payload.token, user: payload.data });

  // Keep legacy session updated for compatibility with existing code
  if (legacySession) {
    legacySession.token = payload.token;
    //legacySession.data = payload.data;
  }

  router.push("/");
}

function onFailure(error: unknown) {
  done();
  state.error = true;
  // Optional: surface server error detail for debugging
  // console.error(error);
}

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

async function submit(e: MouseEvent) {
  e.preventDefault();
  begin();

  try {
    const { email, password } = state.user;
    const resp = await AuthService.login(email, password);
    onLogin(resp as LoginResponse);
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
          <h2 class="auth-title mb-0">Login</h2>
          <span v-if="state.registrationEnabled" class="auth-subtext">
            Need an account?
            <router-link id="to-register" to="/auth/register">Register</router-link>
          </span>
        </div>

        <form class="auth-form" @submit.prevent="submit">
          <div class="form-floating mb-3">
            <input
              id="tb-email"
              v-model="state.user.email"
              class="form-control"
              name="email"
              placeholder="name@example.com"
              required
              type="email"
              autocomplete="email"
            />
            <label for="tb-email">Email</label>
          </div>

          <div class="form-floating mb-3">
            <input
              id="current-password"
              v-model="state.user.password"
              class="form-control"
              name="password"
              placeholder="*****"
              required
              type="password"
              autocomplete="current-password"
            />
            <label for="current-password">Password</label>
          </div>

          <div class="d-flex align-items-center justify-content-between">
            <router-link
              id="to-recover"
              class="auth-link"
              to="/auth/reset"
            >
              Forgot password?
            </router-link>

            <div class="d-flex align-items-center gap-3">
              <Loader v-if="state.waiting" inverse />
              <button
                type="submit"
                class="btn btn-primary btn-lg px-4"
                :disabled="state.waiting"
              >
                Login
              </button>
            </div>
          </div>
        </form>

        <!-- Error message -->
        <div v-if="state.error && !state.waiting" class="error-message">
          <i class="bi bi-exclamation-triangle me-2"></i>
          Incorrect Email/Password combination. Please try again.
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