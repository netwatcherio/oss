<script lang="ts" setup>
import { reactive, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Loader from '@/components/Loader.vue'
import AuthLayout from '@/components/AuthLayout.vue'
import { getSession, setSession } from '@/session'

const route = useRoute()
const router = useRouter()

interface InviteInfo {
  workspace_id: number
  workspace_name: string
  email: string
  role: string
}

interface CompleteResponse {
  token: string
  user: object
  member: object
  workspace_id: number
}

const state = reactive({
  loading: true,
  submitting: false,
  error: '',
  expired: false,
  alreadyUsed: false,
  notFound: false,
  info: null as InviteInfo | null,
  form: {
    name: '',
    password: '',
    confirmPassword: ''
  }
})

const token = computed(() => route.params.token as string)

const passwordsMatch = computed(() => {
  return state.form.password === state.form.confirmPassword
})

const canSubmit = computed(() => {
  return (
    state.form.name.trim().length > 0 &&
    state.form.password.length >= 8 &&
    passwordsMatch.value &&
    !state.submitting
  )
})

async function fetchInviteInfo() {
  state.loading = true
  state.error = ''

  try {
    const baseUrl = import.meta.env.VITE_API_URL || ''
    const resp = await fetch(`${baseUrl}/invite/${token.value}`)
    
    if (resp.status === 404) {
      state.notFound = true
      return
    }
    if (resp.status === 410) {
      state.expired = true
      return
    }
    if (resp.status === 409) {
      state.alreadyUsed = true
      return
    }
    if (!resp.ok) {
      const data = await resp.json()
      state.error = data.error || 'Failed to validate invite'
      return
    }

    state.info = await resp.json()
  } catch (err) {
    state.error = 'Failed to connect to server'
  } finally {
    state.loading = false
  }
}

async function submit(e: Event) {
  e.preventDefault()
  
  if (!canSubmit.value) return
  
  state.submitting = true
  state.error = ''

  try {
    const baseUrl = import.meta.env.VITE_API_URL || ''
    const resp = await fetch(`${baseUrl}/invite/${token.value}/complete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: state.form.name.trim(),
        password: state.form.password
      })
    })

    if (!resp.ok) {
      const data = await resp.json()
      state.error = data.error || 'Failed to complete registration'
      return
    }

    const data: CompleteResponse = await resp.json()
    
    // Save session
    setSession({ token: data.token, user: data.user })
    
    // Redirect to the workspace
    router.push(`/workspaces/${data.workspace_id}`)
  } catch (err) {
    state.error = 'Failed to connect to server'
  } finally {
    state.submitting = false
  }
}

onMounted(() => {
  // If already logged in, redirect home
  const session = getSession()
  if (session?.token) {
    router.push('/')
    return
  }
  
  fetchInviteInfo()
})
</script>

<template>
  <AuthLayout>
    <!-- Loading state -->
    <div v-if="state.loading" class="text-center py-5">
      <Loader large />
      <p class="mt-3 text-muted">Validating invite...</p>
    </div>

    <!-- Error: Not found -->
    <div v-else-if="state.notFound" class="card">
      <div class="card-body text-center py-5">
        <div class="status-icon status-error">
          <i class="bi bi-x-circle"></i>
        </div>
        <h3 class="mt-3">Invalid Invite Link</h3>
        <p class="text-muted">This invite link is not valid or has been revoked.</p>
        <router-link to="/auth/login" class="btn btn-primary mt-3">Go to Login</router-link>
      </div>
    </div>

    <!-- Error: Expired -->
    <div v-else-if="state.expired" class="card">
      <div class="card-body text-center py-5">
        <div class="status-icon status-warning">
          <i class="bi bi-clock-history"></i>
        </div>
        <h3 class="mt-3">Invite Expired</h3>
        <p class="text-muted">This invite link has expired. Please ask for a new invitation.</p>
        <router-link to="/auth/login" class="btn btn-primary mt-3">Go to Login</router-link>
      </div>
    </div>

    <!-- Error: Already used -->
    <div v-else-if="state.alreadyUsed" class="card">
      <div class="card-body text-center py-5">
        <div class="status-icon status-success">
          <i class="bi bi-check-circle"></i>
        </div>
        <h3 class="mt-3">Already Registered</h3>
        <p class="text-muted">This invite has already been used. Please log in with your account.</p>
        <router-link to="/auth/login" class="btn btn-primary mt-3">Go to Login</router-link>
      </div>
    </div>

    <!-- Success: Show form -->
    <div v-else-if="state.info" class="card">
      <div class="card-body">
        <div class="d-flex align-items-end justify-content-between gap-2 mb-3">
          <h2 class="auth-title mb-0">Join {{ state.info.workspace_name }}</h2>
          <span class="auth-subtext">
            Invited as <strong>{{ state.info.email }}</strong>
          </span>
        </div>

        <p class="text-muted small mb-3">
          Complete your registration to join this workspace.
        </p>

        <form @submit="submit">
          <div class="form-floating mb-3">
            <input
              id="name"
              v-model="state.form.name"
              type="text"
              class="form-control"
              placeholder="Your Name"
              required
              autocomplete="name"
            />
            <label for="name">Display Name</label>
          </div>

          <div class="form-floating mb-3">
            <input
              id="password"
              v-model="state.form.password"
              type="password"
              class="form-control"
              :class="{ 'is-invalid': state.form.password && state.form.password.length < 8 }"
              placeholder="Password"
              required
              minlength="8"
              autocomplete="new-password"
            />
            <label for="password">Password</label>
            <div class="form-text" v-if="!state.form.password">
              Must be at least 8 characters
            </div>
            <div class="invalid-feedback" v-if="state.form.password && state.form.password.length < 8">
              Password must be at least 8 characters
            </div>
          </div>

          <div class="form-floating mb-3">
            <input
              id="confirmPassword"
              v-model="state.form.confirmPassword"
              type="password"
              class="form-control"
              :class="{ 'is-invalid': state.form.confirmPassword && !passwordsMatch }"
              placeholder="Confirm Password"
              required
              autocomplete="new-password"
            />
            <label for="confirmPassword">Confirm Password</label>
            <div class="invalid-feedback" v-if="state.form.confirmPassword && !passwordsMatch">
              Passwords do not match
            </div>
          </div>

          <!-- Error message -->
          <div v-if="state.error" class="error-message mb-3">
            <i class="bi bi-exclamation-triangle me-2"></i>
            {{ state.error }}
          </div>

          <div class="d-flex align-items-center justify-content-between">
            <router-link to="/auth/login" class="auth-link">
              Already have an account?
            </router-link>

            <div class="d-flex align-items-center gap-3">
              <Loader v-if="state.submitting" inverse />
              <button
                type="submit"
                class="btn btn-primary btn-lg px-4"
                :disabled="!canSubmit"
              >
                Complete Registration
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>

    <!-- Generic error -->
    <div v-else-if="state.error" class="card">
      <div class="card-body">
        <div class="error-message">
          <i class="bi bi-exclamation-triangle me-2"></i>
          {{ state.error }}
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

.status-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 4rem;
  height: 4rem;
  border-radius: 50%;
  font-size: 2rem;
}

.status-error {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.status-warning {
  background: rgba(234, 179, 8, 0.15);
  color: #eab308;
}

.status-success {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}
</style>
