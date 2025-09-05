<script lang="ts" setup>
import core from "@/core";
import {clearSession, type Session} from "@/session";

const persistent = core.persistent()
const router = core.router()
const session = core.session()

function logout() {
  clearSession()
  router.push("/auth/login")
}
</script>

<template>
  <nav class="navbar">
    <div class="container-fluid">
      <!-- Logo Section -->
      <router-link to="/" class="navbar-brand">
        <i class="fa-solid fa-eye brand-icon"></i>
        <span class="brand-text">netwatcher.io</span>
      </router-link>

      <!-- Right Side Actions -->
      <div class="navbar-actions">
        <!-- Theme Toggle -->
        <button class="nav-icon-btn" title="Toggle theme">
          <i class="fa-solid fa-moon"></i>
        </button>

        <!-- Notifications -->
        <button class="nav-icon-btn" title="Notifications">
          <i class="fa-solid fa-bell"></i>
          <span class="notification-badge">3</span>
        </button>

        <!-- Divider -->
        <div class="nav-divider"></div>

        <!-- User Menu -->
        <div class="user-menu dropdown">
          <button 
            class="user-menu-btn" 
            type="button" 
            data-bs-toggle="dropdown" 
            aria-expanded="false"
          >
            <div class="user-avatar">
              <i class="fa-solid fa-user"></i>
            </div>
            <div class="user-info">
              <span class="user-name">
                {{ session.user?.name }}
              </span>
              <span class="user-role">Administrator</span>
            </div>
            <i class="fa-solid fa-chevron-down dropdown-indicator"></i>
          </button>
          
          <!-- Dropdown Menu -->
          <ul class="dropdown-menu dropdown-menu-end">
            <li>
              <router-link to="/profile" class="dropdown-item">
                <i class="fa-solid fa-user-circle"></i> Profile
              </router-link>
            </li>
            <li>
              <router-link to="/settings" class="dropdown-item">
                <i class="fa-solid fa-cog"></i> Settings
              </router-link>
            </li>
            <li><hr class="dropdown-divider"></li>
            <li>
              <button @click="logout" class="dropdown-item text-danger">
                <i class="fa-solid fa-sign-out-alt"></i> Logout
              </button>
            </li>
          </ul>
        </div>
      </div>
    </div>
  </nav>
</template>

<style scoped>
/* Navbar Base */
.navbar {
  background: #ffffff;
  border-bottom: 1px solid #e5e7eb;
  padding: 0.75rem 0;
  box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.1);
}

.container-fluid {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1.5rem;
}

/* Logo/Brand */
.navbar-brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  text-decoration: none;
  color: #1f2937;
  font-weight: 600;
  font-size: 1.25rem;
  transition: all 0.2s;
}

.navbar-brand:hover {
  color: #3b82f6;
  transform: translateY(-1px);
}

.brand-icon {
  font-size: 1.5rem;
  color: #3b82f6;
  transition: transform 0.2s;
}

.navbar-brand:hover .brand-icon {
  transform: scale(1.1);
}

.brand-text {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  letter-spacing: -0.025em;
}

/* Navbar Actions */
.navbar-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

/* Icon Buttons */
.nav-icon-btn {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.5rem;
  height: 2.5rem;
  border: none;
  background: transparent;
  color: #6b7280;
  border-radius: 8px;
  transition: all 0.2s;
  cursor: pointer;
}

.nav-icon-btn:hover {
  background: #f3f4f6;
  color: #1f2937;
}

.nav-icon-btn i {
  font-size: 1.125rem;
}

/* Notification Badge */
.notification-badge {
  position: absolute;
  top: 6px;
  right: 6px;
  background: #ef4444;
  color: white;
  font-size: 0.625rem;
  font-weight: 600;
  padding: 0.125rem 0.375rem;
  border-radius: 999px;
  min-width: 1.125rem;
  text-align: center;
}

/* Divider */
.nav-divider {
  width: 1px;
  height: 2rem;
  background: #e5e7eb;
  margin: 0 0.5rem;
}

/* User Menu */
.user-menu {
  position: relative;
}

.user-menu-btn {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border: none;
  background: transparent;
  border-radius: 8px;
  transition: all 0.2s;
  cursor: pointer;
}

.user-menu-btn:hover {
  background: #f3f4f6;
}

.user-avatar {
  width: 2.25rem;
  height: 2.25rem;
  background: #3b82f6;
  color: white;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1rem;
}

.user-info {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  text-align: left;
}

.user-name {
  font-size: 0.875rem;
  font-weight: 600;
  color: #1f2937;
  line-height: 1.25;
  text-transform: capitalize;
}

.user-role {
  font-size: 0.75rem;
  color: #6b7280;
  line-height: 1;
}

.dropdown-indicator {
  font-size: 0.75rem;
  color: #9ca3af;
  margin-left: 0.25rem;
}

/* Dropdown Menu */
.dropdown-menu {
  margin-top: 0.5rem;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1);
  padding: 0.5rem;
  min-width: 200px;
}

.dropdown-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.625rem 0.875rem;
  border-radius: 6px;
  font-size: 0.875rem;
  color: #374151;
  transition: all 0.2s;
}

.dropdown-item:hover {
  background: #f3f4f6;
  color: #1f2937;
}

.dropdown-item i {
  width: 1.25rem;
  text-align: center;
  font-size: 1rem;
}

.dropdown-item.text-danger {
  color: #ef4444;
}

.dropdown-item.text-danger:hover {
  background: #fef2f2;
  color: #dc2626;
}

.dropdown-divider {
  margin: 0.5rem 0;
  border-color: #e5e7eb;
}

/* Mobile Responsiveness */
@media (max-width: 576px) {
  .container-fluid {
    padding: 0 1rem;
  }
  
  .brand-text {
    display: none;
  }
  
  .navbar-brand {
    gap: 0;
  }
  
  .brand-icon {
    font-size: 1.75rem;
  }
  
  .user-info {
    display: none;
  }
  
  .dropdown-indicator {
    display: none;
  }
  
  .user-menu-btn {
    padding: 0;
  }
  
  .navbar-actions {
    gap: 0.5rem;
  }
}

@media (max-width: 768px) {
  .user-role {
    display: none;
  }
}

/* Dark Mode Support (optional) */
@media (prefers-color-scheme: dark) {
  .navbar {
    background: #1f2937;
    border-bottom-color: #374151;
  }
  
  .navbar-brand {
    color: #f9fafb;
  }
  
  .navbar-brand:hover {
    color: #60a5fa;
  }
  
  .nav-icon-btn {
    color: #9ca3af;
  }
  
  .nav-icon-btn:hover {
    background: #374151;
    color: #f9fafb;
  }
  
  .nav-divider {
    background: #374151;
  }
  
  .user-menu-btn:hover {
    background: #374151;
  }
  
  .user-name {
    color: #f9fafb;
  }
  
  .user-role {
    color: #9ca3af;
  }
  
  .dropdown-menu {
    background: #1f2937;
    border-color: #374151;
  }
  
  .dropdown-item {
    color: #d1d5db;
  }
  
  .dropdown-item:hover {
    background: #374151;
    color: #f9fafb;
  }
}
</style>