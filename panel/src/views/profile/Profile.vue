<script lang="ts" setup>

import {ref, reactive, onMounted, computed} from "vue";
import core from "@/core";
import Title from "@/components/Title.vue";
import {setSession, getSession} from "@/session";

const session = core.session()

const state = reactive({
  loaded: false,
  saving: false,
  error: '',
  success: false,
  editing: false,
  changingPassword: false,
  passwordSaving: false,
  passwordError: '',
  passwordSuccess: false,
})

const profileForm = reactive({
  name: '',
})

const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
})

// Computed display name - handles the name field properly
const displayName = computed(() => {
  return session?.user?.name || session?.user?.email?.split('@')[0] || 'User'
})

// Get initials for avatar placeholder
const initials = computed(() => {
  const name = displayName.value
  if (!name) return '?'
  const parts = name.split(' ')
  if (parts.length >= 2) {
    return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
  }
  return name.substring(0, 2).toUpperCase()
})

// Format role for display
const displayRole = computed(() => {
  const role = session?.user?.role
  if (role === 'SITE_ADMIN') return 'Site Administrator'
  return 'Standard User'
})

const isSiteAdmin = computed(() => {
  return session?.user?.role === 'SITE_ADMIN'
})

onMounted(() => {
  // Initialize form with current user data
  if (session?.user) {
    profileForm.name = session.user.name || ''
  }
  state.loaded = true
})

function startEditing() {
  profileForm.name = session?.user?.name || ''
  state.editing = true
  state.success = false
  state.error = ''
}

function cancelEditing() {
  state.editing = false
  state.error = ''
}

function openPasswordModal() {
  passwordForm.oldPassword = ''
  passwordForm.newPassword = ''
  passwordForm.confirmPassword = ''
  state.passwordError = ''
  state.passwordSuccess = false
  state.changingPassword = true
}

function closePasswordModal() {
  state.changingPassword = false
  state.passwordError = ''
}

async function updateProfile() {
  state.saving = true
  state.error = ''
  state.success = false
  
  try {
    const response = await fetch('/api/me/profile', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${session?.token}`
      },
      body: JSON.stringify({
        name: profileForm.name,
      })
    })
    
    if (!response.ok) {
      const err = await response.json()
      throw new Error(err.error || 'Failed to update profile')
    }
    
    const updatedUser = await response.json()
    
    // Update local session with new user data
    const currentSession = getSession()
    if (currentSession) {
      setSession({
        ...currentSession,
        user: {
          ...currentSession.user,
          name: updatedUser.name,
        }
      })
    }
    
    state.success = true
    state.editing = false
  } catch (err: any) {
    state.error = err.message
  } finally {
    state.saving = false
  }
}

async function changePassword() {
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    state.passwordError = 'New passwords do not match'
    return
  }
  
  if (passwordForm.newPassword.length < 6) {
    state.passwordError = 'Password must be at least 6 characters'
    return
  }
  
  state.passwordSaving = true
  state.passwordError = ''
  
  try {
    const response = await fetch('/api/auth/me/password', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${session?.token}`
      },
      body: JSON.stringify({
        old_password: passwordForm.oldPassword,
        new_password: passwordForm.newPassword,
      })
    })
    
    if (!response.ok) {
      const err = await response.json()
      throw new Error(err.error || 'Failed to change password')
    }
    
    state.passwordSuccess = true
    setTimeout(() => {
      closePasswordModal()
    }, 1500)
  } catch (err: any) {
    state.passwordError = err.message
  } finally {
    state.passwordSaving = false
  }
}

</script>

<template>
  <Title title="Profile"></Title>
  <div class="container-fluid profile-page">
    <div class="row justify-content-center">
      <div class="col-lg-8 col-xl-6">
        
        <!-- Profile Header Card -->
        <div class="card profile-header-card">
          <div class="card-body text-center py-4">
            <div class="profile-avatar">
              <span class="avatar-initials">{{ initials }}</span>
            </div>
            <h3 class="profile-name mt-3 mb-1">{{ displayName }}</h3>
            <p class="profile-email text-muted mb-3">{{ session?.user?.email }}</p>
            <div class="profile-badges">
              <span v-if="session?.user?.verified" class="badge badge-verified">
                <i class="ri-checkbox-circle-line me-1"></i> Verified
              </span>
              <span v-else class="badge badge-unverified">
                <i class="ri-error-warning-line me-1"></i> Not Verified
              </span>
              <span v-if="isSiteAdmin" class="badge badge-admin">
                <i class="ri-shield-star-line me-1"></i> Site Admin
              </span>
            </div>
          </div>
        </div>

        <!-- Success Message -->
        <div v-if="state.success" class="alert alert-success d-flex align-items-center" role="alert">
          <i class="ri-checkbox-circle-fill me-2"></i>
          <span>Profile updated successfully!</span>
        </div>

        <!-- Account Information Card -->
        <div class="card settings-card">
          <div class="card-header d-flex justify-content-between align-items-center">
            <div>
              <h5 class="mb-0"><i class="ri-user-line me-2"></i>Account Information</h5>
              <small class="text-muted">Manage your personal details</small>
            </div>
            <button 
              v-if="!state.editing" 
              class="btn btn-sm btn-outline-primary" 
              @click="startEditing"
            >
              <i class="ri-edit-line me-1"></i> Edit
            </button>
          </div>
          <div class="card-body">
            <!-- View Mode -->
            <div v-if="!state.editing">
              <div class="info-row">
                <div class="info-label">
                  <i class="ri-user-3-line"></i>
                  Display Name
                </div>
                <div class="info-value">{{ displayName }}</div>
              </div>
              <div class="info-row">
                <div class="info-label">
                  <i class="ri-mail-line"></i>
                  Email Address
                </div>
                <div class="info-value">{{ session?.user?.email }}</div>
              </div>
              <div class="info-row">
                <div class="info-label">
                  <i class="ri-shield-check-line"></i>
                  Account Status
                </div>
                <div class="info-value">
                  <span v-if="session?.user?.verified" class="status-active">
                    <i class="ri-checkbox-circle-fill"></i> Active
                  </span>
                  <span v-else class="status-pending">
                    <i class="ri-time-line"></i> Pending Verification
                  </span>
                </div>
              </div>
              <div class="info-row">
                <div class="info-label">
                  <i class="ri-user-star-line"></i>
                  Account Type
                </div>
                <div class="info-value">
                  <span v-if="isSiteAdmin" class="text-primary">
                    <i class="ri-shield-star-line me-1"></i>{{ displayRole }}
                  </span>
                  <span v-else>{{ displayRole }}</span>
                </div>
              </div>
            </div>

            <!-- Edit Mode -->
            <form v-else @submit.prevent="updateProfile">
              <div class="mb-3">
                <label class="form-label">Display Name</label>
                <input 
                  type="text" 
                  class="form-control" 
                  v-model="profileForm.name"
                  placeholder="Enter your name"
                />
                <small class="text-muted">This is how your name will appear across the platform</small>
              </div>
              <div class="mb-3">
                <label class="form-label">Email Address</label>
                <input 
                  type="email" 
                  class="form-control" 
                  :value="session?.user?.email"
                  disabled
                />
                <small class="text-muted">Email address cannot be changed</small>
              </div>
              
              <div v-if="state.error" class="alert alert-danger py-2">
                <i class="ri-error-warning-line me-1"></i> {{ state.error }}
              </div>
              
              <div class="d-flex gap-2 mt-4">
                <button type="submit" class="btn btn-primary" :disabled="state.saving">
                  <span v-if="state.saving">
                    <span class="spinner-border spinner-border-sm me-2"></span>
                    Saving...
                  </span>
                  <span v-else>
                    <i class="ri-save-line me-1"></i> Save Changes
                  </span>
                </button>
                <button type="button" class="btn btn-outline-secondary" @click="cancelEditing" :disabled="state.saving">
                  Cancel
                </button>
              </div>
            </form>
          </div>
        </div>

        <!-- Security Card -->
        <div class="card settings-card">
          <div class="card-header">
            <h5 class="mb-0"><i class="ri-lock-line me-2"></i>Security</h5>
            <small class="text-muted">Manage your account security</small>
          </div>
          <div class="card-body">
            <div class="security-item">
              <div class="security-info">
                <div class="security-label">
                  <i class="ri-key-2-line"></i>
                  Password
                </div>
                <div class="security-description text-muted">
                  Change your password to keep your account secure
                </div>
              </div>
              <button class="btn btn-sm btn-outline-secondary" @click="openPasswordModal">
                Change Password
              </button>
            </div>
          </div>
        </div>

      </div>
    </div>
  </div>

  <!-- Password Change Modal -->
  <div v-if="state.changingPassword" class="modal-backdrop fade show"></div>
  <div v-if="state.changingPassword" class="modal fade show d-block" tabindex="-1">
    <div class="modal-dialog modal-dialog-centered">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title"><i class="ri-lock-password-line me-2"></i>Change Password</h5>
          <button type="button" class="btn-close" @click="closePasswordModal"></button>
        </div>
        <form @submit.prevent="changePassword">
          <div class="modal-body">
            <div v-if="state.passwordSuccess" class="alert alert-success">
              <i class="ri-checkbox-circle-fill me-2"></i>Password changed successfully!
            </div>
            <div v-else>
              <div class="mb-3">
                <label class="form-label">Current Password</label>
                <input 
                  type="password" 
                  class="form-control" 
                  v-model="passwordForm.oldPassword"
                  placeholder="Enter current password"
                  required
                />
              </div>
              <div class="mb-3">
                <label class="form-label">New Password</label>
                <input 
                  type="password" 
                  class="form-control" 
                  v-model="passwordForm.newPassword"
                  placeholder="Enter new password"
                  required
                />
              </div>
              <div class="mb-3">
                <label class="form-label">Confirm New Password</label>
                <input 
                  type="password" 
                  class="form-control" 
                  v-model="passwordForm.confirmPassword"
                  placeholder="Confirm new password"
                  required
                />
              </div>
              <div v-if="state.passwordError" class="alert alert-danger py-2 mb-0">
                <i class="ri-error-warning-line me-1"></i> {{ state.passwordError }}
              </div>
            </div>
          </div>
          <div class="modal-footer" v-if="!state.passwordSuccess">
            <button type="button" class="btn btn-outline-secondary" @click="closePasswordModal">
              Cancel
            </button>
            <button type="submit" class="btn btn-primary" :disabled="state.passwordSaving">
              <span v-if="state.passwordSaving">
                <span class="spinner-border spinner-border-sm me-2"></span>
                Saving...
              </span>
              <span v-else>Change Password</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.profile-page {
  padding-top: 0;
  padding-bottom: 2rem;
}

/* Profile Header Card */
.profile-header-card {
  background: linear-gradient(135deg, var(--bs-primary) 0%, #4a90d9 100%);
  border: none;
  border-radius: 16px;
  margin-bottom: 1.5rem;
  overflow: hidden;
}

.profile-header-card .card-body {
  position: relative;
}

.profile-avatar {
  width: 88px;
  height: 88px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.2);
  backdrop-filter: blur(10px);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 3px solid rgba(255, 255, 255, 0.4);
}

.avatar-initials {
  font-size: 2rem;
  font-weight: 600;
  color: white;
  text-transform: uppercase;
}

.profile-name {
  color: white;
  font-weight: 600;
}

.profile-email {
  color: rgba(255, 255, 255, 0.85) !important;
}

.profile-badges {
  display: flex;
  gap: 0.5rem;
  justify-content: center;
  flex-wrap: wrap;
}

.badge {
  padding: 0.5rem 0.875rem;
  border-radius: 50px;
  font-weight: 500;
  font-size: 0.8rem;
}

.badge-verified {
  background: rgba(255, 255, 255, 0.2);
  color: white;
}

.badge-unverified {
  background: rgba(255, 193, 7, 0.3);
  color: #fff3cd;
}

.badge-admin {
  background: rgba(255, 255, 255, 0.15);
  color: white;
  border: 1px solid rgba(255, 255, 255, 0.3);
}

/* Settings Cards */
.settings-card {
  border: none;
  border-radius: 12px;
  margin-bottom: 1rem;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.settings-card .card-header {
  background: transparent;
  border-bottom: 1px solid var(--bs-border-color);
  padding: 1.25rem 1.5rem;
}

.settings-card .card-header h5 {
  display: flex;
  align-items: center;
  font-weight: 600;
}

.settings-card .card-header h5 i {
  opacity: 0.7;
}

.settings-card .card-body {
  padding: 1.5rem;
}

/* Info Rows */
.info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 0;
  border-bottom: 1px solid var(--bs-border-color);
}

.info-row:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.info-row:first-child {
  padding-top: 0;
}

.info-label {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: var(--bs-secondary-color);
  font-size: 0.9rem;
}

.info-label i {
  font-size: 1.1rem;
  opacity: 0.6;
}

.info-value {
  font-weight: 500;
  color: var(--bs-body-color);
}

.status-active {
  color: var(--bs-success);
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.status-pending {
  color: var(--bs-warning);
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

/* Security Section */
.security-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.security-info {
  flex: 1;
}

.security-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 500;
  margin-bottom: 0.25rem;
}

.security-description {
  font-size: 0.85rem;
}

/* Form Styling */
.form-control {
  border-radius: 8px;
  padding: 0.625rem 1rem;
}

.form-control:focus {
  box-shadow: 0 0 0 3px rgba(var(--bs-primary-rgb), 0.15);
}

.form-label {
  font-weight: 500;
  margin-bottom: 0.5rem;
}

/* Button improvements */
.btn {
  border-radius: 8px;
  padding: 0.5rem 1rem;
  font-weight: 500;
}

.btn-primary {
  box-shadow: 0 2px 4px rgba(var(--bs-primary-rgb), 0.3);
}

.btn-outline-primary:hover {
  box-shadow: 0 2px 4px rgba(var(--bs-primary-rgb), 0.2);
}

/* Alert styling */
.alert {
  border-radius: 10px;
  border: none;
  margin-bottom: 1rem;
}

.alert-success {
  background: rgba(var(--bs-success-rgb), 0.1);
  color: var(--bs-success);
}

.alert-danger {
  background: rgba(var(--bs-danger-rgb), 0.1);
  color: var(--bs-danger);
}

/* Modal styling */
.modal-content {
  border: none;
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
}

.modal-header {
  border-bottom: 1px solid var(--bs-border-color);
  padding: 1.25rem 1.5rem;
}

.modal-title {
  display: flex;
  align-items: center;
  font-weight: 600;
}

.modal-body {
  padding: 1.5rem;
}

.modal-footer {
  border-top: 1px solid var(--bs-border-color);
  padding: 1rem 1.5rem;
}

/* Responsive adjustments */
@media (max-width: 576px) {
  .info-row {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.5rem;
  }
  
  .security-item {
    flex-direction: column;
    align-items: flex-start;
  }
  
  .security-item .btn {
    margin-top: 0.5rem;
  }
  
  .profile-badges {
    flex-direction: column;
    align-items: center;
  }
}
</style>