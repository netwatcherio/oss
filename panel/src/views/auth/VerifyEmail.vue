<script lang="ts" setup>
import { reactive, onMounted } from "vue";
import { useRoute } from "vue-router";
import Loader from "@/components/Loader.vue";
import AuthLayout from "@/components/AuthLayout.vue";
import { AuthService } from "@/services/apiService";

const route = useRoute();

const state = reactive({
  loading: true,
  success: false,
  error: false,
  errorMessage: "",
  expired: false,
});

onMounted(async () => {
  const token = route.params.token as string;
  if (!token) {
    state.loading = false;
    state.error = true;
    state.errorMessage = "Invalid verification link. No token provided.";
    return;
  }

  try {
    await AuthService.verifyEmail(token);
    state.loading = false;
    state.success = true;
  } catch (err: any) {
    state.loading = false;
    state.error = true;

    const serverError = err?.response?.data?.error ?? "";
    if (serverError.includes("expired")) {
      state.expired = true;
      state.errorMessage = "This verification link has expired. Please request a new one from your profile.";
    } else if (serverError.includes("not found")) {
      state.errorMessage = "This verification link is invalid or has already been used.";
    } else {
      state.errorMessage = "Unable to verify email. Please try again later.";
    }
  }
});
</script>

<template>
  <AuthLayout>
    <div class="card">
      <div class="card-body">
        <h2 class="auth-title mb-3">Email Verification</h2>

        <!-- Loading -->
        <div v-if="state.loading" class="status-content">
          <Loader inverse />
          <p class="text-muted mt-3">Verifying your email address...</p>
        </div>

        <!-- Success -->
        <div v-else-if="state.success" class="status-content">
          <div class="status-icon success-icon">
            <i class="bi bi-check-circle"></i>
          </div>
          <h4 class="mt-3">Email Verified!</h4>
          <p class="text-muted mb-4">
            Your email address has been verified successfully. You now have full access to NetWatcher.
          </p>
          <router-link to="/" class="btn btn-primary">
            Go to Dashboard
          </router-link>
        </div>

        <!-- Error -->
        <div v-else-if="state.error" class="status-content">
          <div class="status-icon error-icon">
            <i class="bi" :class="state.expired ? 'bi-clock-history' : 'bi-x-circle'"></i>
          </div>
          <h4 class="mt-3">{{ state.expired ? 'Link Expired' : 'Verification Failed' }}</h4>
          <p class="text-muted mb-4">{{ state.errorMessage }}</p>
          <div class="d-flex gap-2 justify-content-center">
            <router-link to="/auth/login" class="btn btn-outline-secondary">
              Back to Login
            </router-link>
          </div>
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
  width: 4rem;
  height: 4rem;
  border-radius: 50%;
  font-size: 2rem;
}

.success-icon {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}

.error-icon {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}
</style>
