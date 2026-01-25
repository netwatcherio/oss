<script setup lang="ts">
/**
 * AuthLayout - Shared layout wrapper for authentication pages
 * Provides consistent branding, centered card layout, and styling
 * Supports both dark and light theme modes via data-theme attribute
 */
</script>

<template>
  <div class="auth-layout">
    <div class="auth-container">
      <!-- Branding Header -->
      <div class="auth-brand">
        <router-link to="/" class="brand-link">
          <i class="bi bi-eye brand-icon"></i>
          <span class="brand-text">netwatcher.io</span>
        </router-link>
      </div>

      <!-- Content Slot -->
      <div class="auth-content">
        <slot></slot>
      </div>

      <!-- Footer -->
      <div class="auth-footer">
        <span class="text-muted small">
          © {{ new Date().getFullYear() }} NetWatcher.io
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.auth-layout {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem 1rem;
  transition: background 0.3s ease;
}

.auth-container {
  width: 100%;
  max-width: 28rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.auth-brand {
  text-align: center;
  margin-bottom: 0.5rem;
}

.brand-link {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  text-decoration: none;
  font-weight: 600;
  font-size: 1.5rem;
  transition: opacity 0.2s ease;
}

.brand-link:hover {
  opacity: 0.85;
}

.brand-icon {
  font-size: 1.75rem;
  background: linear-gradient(135deg, #3b82f6 0%, #10b981 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.auth-content {
  animation: fadeSlideUp 0.4s ease-out;
}

@keyframes fadeSlideUp {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.auth-footer {
  text-align: center;
  opacity: 0.7;
}

/* ==================== DARK MODE ==================== */
/* Use :deep() to scope styles to children of this component only */
:deep([data-theme="dark"]) .auth-layout,
.auth-layout:where([data-theme="dark"] *) {
  background: linear-gradient(135deg, 
    #0f172a 0%, 
    color-mix(in srgb, var(--bs-primary) 12%, #0f172a) 50%,
    #0f172a 100%
  );
}
</style>

<style>
/* Scoped auth-specific styles that only apply within .auth-layout */
.auth-layout {
  /* Dark mode - applied via parent selector */
}

[data-theme="dark"] .auth-layout {
  background: linear-gradient(135deg, 
    #0f172a 0%, 
    color-mix(in srgb, var(--bs-primary) 12%, #0f172a) 50%,
    #0f172a 100%
  );
}

[data-theme="dark"] .auth-layout .brand-link {
  color: white;
}

[data-theme="dark"] .auth-layout .brand-text {
  background: linear-gradient(135deg, white 0%, #93c5fd 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

[data-theme="dark"] .auth-layout .card {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 1rem;
  background: rgba(30, 41, 59, 0.95);
  backdrop-filter: blur(10px);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
}

[data-theme="dark"] .auth-layout .card-body {
  padding: 1.5rem;
}

[data-theme="dark"] .auth-layout .form-control {
  background-color: rgba(15, 23, 42, 0.6);
  border-color: rgba(255, 255, 255, 0.15);
  color: white;
  border-radius: 0.5rem;
}

[data-theme="dark"] .auth-layout .form-control::placeholder {
  color: rgba(255, 255, 255, 0.4);
}

[data-theme="dark"] .auth-layout .form-control:focus {
  background-color: rgba(15, 23, 42, 0.8);
  border-color: var(--bs-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.25);
  color: white;
}

[data-theme="dark"] .auth-layout .form-floating label {
  color: rgba(255, 255, 255, 0.6);
}

[data-theme="dark"] .auth-layout .error-message {
  background-color: rgba(239, 68, 68, 0.15);
  border: 1px solid rgba(239, 68, 68, 0.5);
  color: #fca5a5;
}

[data-theme="dark"] .auth-layout .success-message {
  background-color: rgba(34, 197, 94, 0.15);
  border: 1px solid rgba(34, 197, 94, 0.5);
  color: #86efac;
}

/* ==================== LIGHT MODE ==================== */
[data-theme="light"] .auth-layout {
  background: linear-gradient(135deg, 
    #f8fafc 0%, 
    color-mix(in srgb, var(--bs-primary) 8%, #f1f5f9) 50%,
    #f8fafc 100%
  );
}

[data-theme="light"] .auth-layout .brand-link {
  color: #1e293b;
}

[data-theme="light"] .auth-layout .brand-text {
  background: linear-gradient(135deg, #1e293b 0%, var(--bs-primary) 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

[data-theme="light"] .auth-layout .card {
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 1rem;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(10px);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
}

[data-theme="light"] .auth-layout .card-body {
  padding: 1.5rem;
}

[data-theme="light"] .auth-layout .form-control {
  background-color: #f8fafc;
  border-color: #e2e8f0;
  color: #1e293b;
  border-radius: 0.5rem;
}

[data-theme="light"] .auth-layout .form-control:focus {
  background-color: white;
  border-color: var(--bs-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

[data-theme="light"] .auth-layout .error-message {
  background-color: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  color: #dc2626;
}

[data-theme="light"] .auth-layout .success-message {
  background-color: rgba(34, 197, 94, 0.1);
  border: 1px solid rgba(34, 197, 94, 0.3);
  color: #16a34a;
}

/* ==================== SHARED AUTH STYLES ==================== */
.auth-layout .btn-primary {
  border-radius: 0.5rem;
  font-weight: 500;
  transition: all 0.2s ease;
}

.auth-layout .btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(var(--bs-primary-rgb), 0.4);
}

.auth-layout .error-message,
.auth-layout .success-message {
  margin-top: 0.5rem;
  padding: 0.75rem 1rem;
  border-radius: 0.5rem;
  animation: authShakeIn 0.3s ease-out;
}

@keyframes authShakeIn {
  0%, 100% { transform: translateX(0); }
  20%, 60% { transform: translateX(-4px); }
  40%, 80% { transform: translateX(4px); }
}
</style>
