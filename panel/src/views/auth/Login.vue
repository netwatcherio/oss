<script lang="ts" setup>
import { reactive } from "vue";
import core from "@/core";
import Loader from "@/components/Loader.vue";
import FormElement from "@/components/FormElement.vue";

// NEW imports: use AuthService + token helpers from the new stack
import { AuthService } from "@/services/apiService";
import { setSession, getSession } from "@/session";

const state = reactive({
  user: {} as User,
  began: 0 as number,
  waiting: false,
  error: false
});

const router = core.router();

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
    legacySession.data = payload.data;
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
  <div class="d-flex justify-content-center align-items-center" style="height: 75vh">
    <div class="form-entry w-100">
      <FormElement title="Login">
        <template #alternate>
          <span class="label-subtext mb-1">
            Need an account?
            <router-link id="to-register" to="/auth/register">register</router-link>
          </span>
        </template>

        <template #body>
          <div class="form-horizontal needs-validation mt-2">
            <div class="form-floating mb-2">
              <input
                  id="tb-email"
                  v-model="state.user.email"
                  class="form-control form-input-bg"
                  name="email"
                  placeholder="name@example.com"
                  required
                  type="email"
              />
              <label for="tb-email">Email</label>
              <div class="invalid-feedback">email is required</div>
            </div>

            <div class="form-floating mb-2">
              <input
                  id="current-password"
                  v-model="state.user.password"
                  class="form-control form-input-bg"
                  name="password"
                  placeholder="*****"
                  required
                  type="password"
              />
              <label for="current-password">Password</label>
              <div class="invalid-feedback">password is required</div>
            </div>

            <div class="d-flex align-items-center justify-content-between">
              <div>
                <router-link
                    id="to-recover"
                    class="label-c4 label-w600 label-underlined px-1"
                    to="/auth/reset"
                >
                  forgot password?
                </router-link>
              </div>

              <div class="d-flex align-items-center gap-3">
                <div class="d-flex align-items-center justify-content-center">
                  <Loader v-if="state.waiting" inverse large />
                </div>

                <button
                    class="btn btn-primary btn-lg px-4 d-flex align-items-center gap-1"
                    @click="submit"
                    :disabled="state.waiting"
                >
                  login
                </button>
              </div>
            </div>
          </div>
        </template>
      </FormElement>

      <div class="" style="height: 4rem">
        <div
            class="mt-2"
            v-if="state.error"
            :class="`${state.waiting ? 'error-message-pending' : 'error-message-animation message-body'}`"
        >
          <div class="text-danger" v-if="!state.waiting">
            Incorrect Email/Password combination. Please try again.
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.form-entry { max-width: 26rem; width: 100%; }
.card { border-radius: 0.8rem; }
.card-body { padding: 0.8rem; }

.error-message-pending { animation: animate-pending 100ms forwards ease-out; }
@keyframes animate-pending {
  0% { filter: saturate(90%) blur(1px); height: 100%; }
  100% { filter: saturate(50%) blur(8px); opacity: 0; height: 0%; }
}

.message-body {
  margin-bottom: 0.375rem;
  background-color: rgba(255, 64, 64, 0.3);
  border: 1px solid rgba(255, 64, 64, 1);
  width: 100%;
  padding: 0.5rem 1rem;
  display: flex;
  align-items: center;
  border-radius: 0.8rem;
}
.error-message-animation { animation: animate-expand-vertical 250ms forwards ease-out; }
@keyframes animate-expand-vertical {
  0% { scale: 0.95; opacity: 0.8; filter: blur(2px); }
  100% { scale: 1; opacity: 1; filter: blur(0px); }
}
.btn-primary.btn-lg { border-radius: 0.375rem !important; }
</style>