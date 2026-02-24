<script lang="ts" setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import core from "@/core";
import {clearSession, type Session} from "@/session";
import { themeService, type Theme } from "@/services/themeService";
import { AlertService } from "@/services/apiService";

const persistent = core.persistent()
const router = core.router()
const session = core.session()

const currentTheme = ref<Theme>(themeService.getTheme());
const alertCount = ref(0);
const isSiteAdmin = computed(() => session.user?.role === 'SITE_ADMIN');
const mobileMenuOpen = ref(false);
const isScrolled = ref(false);

let unsubscribe: (() => void) | null = null;
let alertPollInterval: ReturnType<typeof setInterval> | null = null;

async function fetchAlertCount() {
  try {
    alertCount.value = await AlertService.getCount();
  } catch (e) {
    // Silent fail - user may not be authenticated yet
  }
}

onMounted(() => {
  unsubscribe = themeService.onThemeChange((theme) => {
    currentTheme.value = theme;
  });
  // Fetch initial alert count
  fetchAlertCount();
  // Poll every 30 seconds for updates
  alertPollInterval = setInterval(fetchAlertCount, 30000);
  
  // Track scroll for navbar styling
  window.addEventListener('scroll', handleScroll);
});

onUnmounted(() => {
  if (unsubscribe) unsubscribe();
  if (alertPollInterval) clearInterval(alertPollInterval);
  window.removeEventListener('scroll', handleScroll);
});

function handleScroll() {
  isScrolled.value = window.scrollY > 10;
}

function toggleTheme() {
  themeService.toggle();
}

function toggleMobileMenu() {
  mobileMenuOpen.value = !mobileMenuOpen.value;
  // Prevent body scroll when menu is open
  document.body.style.overflow = mobileMenuOpen.value ? 'hidden' : '';
}

function closeMobileMenu() {
  mobileMenuOpen.value = false;
  document.body.style.overflow = '';
}

function logout() {
  closeMobileMenu();
  clearSession()
  router.push("/auth/login")
}

// Navigation items for mobile menu
const navItems = computed(() => [
  { to: '/', icon: 'bi-house', label: 'Home' },
  { to: '/workspaces', icon: 'bi-grid', label: 'Workspaces' },
  { to: '/lookup', icon: 'bi-search', label: 'IP Lookup' },
  { to: '/workspaces/alerts', icon: 'bi-bell', label: 'Alerts', badge: alertCount.value },
  ...(isSiteAdmin.value ? [{ to: '/admin', icon: 'bi-shield-lock', label: 'Admin' }] : []),
]);
</script>

<template>
  <nav class="navbar" :class="{ 'scrolled': isScrolled, 'mobile-open': mobileMenuOpen }">
    <div class="container-fluid">
      <!-- Logo Section -->
      <router-link to="/" class="navbar-brand">
        <i class="bi bi-eye brand-icon"></i>
        <span class="brand-text">netwatcher.io</span>
      </router-link>

      <!-- Desktop Actions -->
      <div class="navbar-actions d-none d-lg-flex">
        <!-- Theme Toggle -->
        <button class="nav-icon-btn" title="Toggle theme" @click="toggleTheme">
          <i :class="currentTheme === 'dark' ? 'bi bi-sun' : 'bi bi-moon'"></i>
        </button>

        <!-- IP Lookup -->
        <router-link to="/lookup" class="nav-icon-btn" title="IP/WHOIS Lookup">
          <i class="bi bi-search"></i>
        </router-link>

        <!-- Alerts -->
        <router-link to="/workspaces/alerts" class="nav-icon-btn" title="Alerts">
          <i class="bi bi-bell"></i>
          <span v-if="alertCount > 0" class="notification-badge">{{ alertCount > 99 ? '99+' : alertCount }}</span>
        </router-link>

        <!-- Admin Panel (site admins only) -->
        <router-link v-if="isSiteAdmin" to="/admin" class="nav-icon-btn admin-btn" title="Admin Panel">
          <i class="bi bi-shield-lock"></i>
        </router-link>

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
              <i class="bi bi-person"></i>
            </div>
            <div class="user-info">
              <span class="user-name">
                {{ session.user?.name }}
              </span>
              <span class="user-role">Administrator</span>
            </div>
            <i class="bi bi-chevron-down dropdown-indicator"></i>
          </button>
          
          <!-- Dropdown Menu -->
          <ul class="dropdown-menu dropdown-menu-end">
            <li>
              <router-link to="/profile" class="dropdown-item">
                <i class="bi bi-gear"></i> Settings
              </router-link>
            </li>
            <li><hr class="dropdown-divider"></li>
            <li>
              <button @click="logout" class="dropdown-item text-danger">
                <i class="bi bi-box-arrow-right"></i> Logout
              </button>
            </li>
          </ul>
        </div>
      </div>
      
      <!-- Mobile Menu Toggle -->
      <button 
        class="mobile-menu-toggle d-lg-none" 
        @click="toggleMobileMenu"
        :aria-label="mobileMenuOpen ? 'Close menu' : 'Open menu'"
        :aria-expanded="mobileMenuOpen"
      >
        <div class="hamburger" :class="{ 'active': mobileMenuOpen }">
          <span></span>
          <span></span>
          <span></span>
        </div>
      </button>
    </div>
    
    <!-- Mobile Menu Drawer -->
    <Transition name="slide-down">
      <div v-if="mobileMenuOpen" class="mobile-drawer d-lg-none">
        <div class="mobile-drawer-content">
          <!-- Mobile User Info -->
          <div class="mobile-user-section">
            <div class="user-avatar mobile">
              <i class="bi bi-person"></i>
            </div>
            <div class="mobile-user-info">
              <span class="mobile-user-name">{{ session.user?.name }}</span>
              <span class="mobile-user-role">Administrator</span>
            </div>
          </div>
          
          <hr class="mobile-divider">
          
          <!-- Mobile Nav Items -->
          <nav class="mobile-nav">
            <router-link 
              v-for="item in navItems" 
              :key="item.to"
              :to="item.to" 
              class="mobile-nav-item"
              @click="closeMobileMenu"
            >
              <i :class="item.icon"></i>
              <span>{{ item.label }}</span>
              <span v-if="item.badge" class="mobile-badge">{{ item.badge > 99 ? '99+' : item.badge }}</span>
            </router-link>
          </nav>
          
          <hr class="mobile-divider">
          
          <!-- Mobile Actions -->
          <div class="mobile-actions">
            <button class="mobile-action-btn" @click="toggleTheme">
              <i :class="currentTheme === 'dark' ? 'bi bi-sun' : 'bi bi-moon'"></i>
              <span>{{ currentTheme === 'dark' ? 'Light Mode' : 'Dark Mode' }}</span>
            </button>
            
            <router-link to="/profile" class="mobile-action-btn" @click="closeMobileMenu">
              <i class="bi bi-gear"></i>
              <span>Settings</span>
            </router-link>
          </div>
          
          <hr class="mobile-divider">
          
          <!-- Mobile Logout -->
          <button class="mobile-logout" @click="logout">
            <i class="bi bi-box-arrow-right"></i>
            <span>Logout</span>
          </button>
        </div>
      </div>
    </Transition>
    
    <!-- Mobile Overlay -->
    <Transition name="fade">
      <div 
        v-if="mobileMenuOpen" 
        class="mobile-overlay d-lg-none"
        @click="closeMobileMenu"
      ></div>
    </Transition>
  </nav>
</template>

<style scoped>
/* Navbar Base */
.navbar {
  position: sticky;
  top: 0;
  z-index: 1030;
  background: var(--bs-body-bg);
  border-bottom: 1px solid var(--bs-border-color);
  padding: 0.75rem 0;
  box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.1);
  transition: all 0.2s ease;
}

.navbar.scrolled {
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
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
  color: var(--bs-body-color);
  font-weight: 600;
  font-size: 1.25rem;
  transition: all 0.2s;
}

.navbar-brand:hover {
  color: var(--bs-primary);
  transform: translateY(-1px);
}

.brand-icon {
  font-size: 1.5rem;
  color: var(--bs-primary);
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
  width: 44px;
  height: 44px;
  min-width: 44px;
  min-height: 44px;
  border: none;
  background: transparent;
  color: var(--bs-secondary-color);
  border-radius: 8px;
  transition: all 0.2s;
  cursor: pointer;
}

.nav-icon-btn:hover {
  background: var(--bs-tertiary-bg);
  color: var(--bs-body-color);
}

.nav-icon-btn i {
  font-size: 1.125rem;
}

/* Admin Button */
.admin-btn {
  color: #8b5cf6;
}

.admin-btn:hover {
  background: rgba(139, 92, 246, 0.1);
  color: #7c3aed;
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
  background: var(--bs-border-color);
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
  background: var(--bs-tertiary-bg);
}

.user-avatar {
  width: 2.25rem;
  height: 2.25rem;
  min-width: 2.25rem;
  background: var(--bs-primary);
  color: white;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1rem;
}

.user-avatar.mobile {
  width: 3rem;
  height: 3rem;
  font-size: 1.25rem;
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
  color: var(--bs-body-color);
  line-height: 1.25;
  text-transform: capitalize;
}

.user-role {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  line-height: 1;
}

.dropdown-indicator {
  font-size: 0.75rem;
  color: var(--bs-secondary-color);
  margin-left: 0.25rem;
}

/* Dropdown Menu */
.dropdown-menu {
  margin-top: 0.5rem;
  border: 1px solid var(--bs-border-color);
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
  color: var(--bs-body-color);
  transition: all 0.2s;
}

.dropdown-item:hover {
  background: var(--bs-tertiary-bg);
  color: var(--bs-body-color);
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
  border-color: var(--bs-border-color);
}

/* Mobile Menu Toggle */
.mobile-menu-toggle {
  width: 48px;
  height: 48px;
  min-width: 48px;
  min-height: 48px;
  border: none;
  background: transparent;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  z-index: 1031;
}

.hamburger {
  width: 24px;
  height: 18px;
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.hamburger span {
  display: block;
  width: 100%;
  height: 2px;
  background: var(--bs-body-color);
  border-radius: 2px;
  transition: all 0.3s ease;
  transform-origin: center;
}

.hamburger.active span:nth-child(1) {
  transform: translateY(8px) rotate(45deg);
}

.hamburger.active span:nth-child(2) {
  opacity: 0;
  transform: scaleX(0);
}

.hamburger.active span:nth-child(3) {
  transform: translateY(-8px) rotate(-45deg);
}

/* Mobile Drawer */
.mobile-drawer {
  position: fixed;
  top: 65px;
  left: 0;
  right: 0;
  bottom: 0;
  background: var(--bs-body-bg);
  z-index: 1030;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

.mobile-drawer-content {
  padding: 1.5rem;
  max-width: 400px;
  margin: 0 auto;
}

.mobile-user-section {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding-bottom: 1rem;
}

.mobile-user-info {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.mobile-user-name {
  font-size: 1.125rem;
  font-weight: 600;
  color: var(--bs-body-color);
}

.mobile-user-role {
  font-size: 0.875rem;
  color: var(--bs-secondary-color);
}

.mobile-divider {
  border: none;
  border-top: 1px solid var(--bs-border-color);
  margin: 1rem 0;
}

.mobile-nav {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.mobile-nav-item {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  color: var(--bs-body-color);
  text-decoration: none;
  border-radius: 12px;
  font-size: 1rem;
  font-weight: 500;
  transition: all 0.2s;
}

.mobile-nav-item:hover,
.mobile-nav-item.router-link-active {
  background: var(--bs-primary);
  color: white;
}

.mobile-nav-item i {
  font-size: 1.25rem;
  width: 1.5rem;
  text-align: center;
}

.mobile-badge {
  margin-left: auto;
  background: #ef4444;
  color: white;
  font-size: 0.75rem;
  font-weight: 600;
  padding: 0.25rem 0.5rem;
  border-radius: 999px;
  min-width: 1.5rem;
  text-align: center;
}

.mobile-actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.mobile-action-btn {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: transparent;
  border: none;
  color: var(--bs-body-color);
  font-size: 1rem;
  font-weight: 500;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
  text-decoration: none;
}

.mobile-action-btn:hover {
  background: var(--bs-tertiary-bg);
}

.mobile-action-btn i {
  font-size: 1.25rem;
  width: 1.5rem;
  text-align: center;
}

.mobile-logout {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem;
  background: rgba(239, 68, 68, 0.1);
  border: none;
  color: #ef4444;
  font-size: 1rem;
  font-weight: 500;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
  width: 100%;
}

.mobile-logout:hover {
  background: rgba(239, 68, 68, 0.2);
}

.mobile-logout i {
  font-size: 1.25rem;
  width: 1.5rem;
  text-align: center;
}

/* Mobile Overlay */
.mobile-overlay {
  position: fixed;
  top: 65px;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 1029;
  backdrop-filter: blur(4px);
}

/* Transitions */
.slide-down-enter-active,
.slide-down-leave-active {
  transition: all 0.3s ease;
}

.slide-down-enter-from,
.slide-down-leave-to {
  opacity: 0;
  transform: translateY(-20px);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

/* Reduced Motion */
@media (prefers-reduced-motion: reduce) {
  .mobile-menu-toggle,
  .hamburger span,
  .navbar,
  .nav-icon-btn,
  .mobile-nav-item,
  .mobile-action-btn,
  .mobile-logout {
    transition: none !important;
  }
  
  .slide-down-enter-active,
  .slide-down-leave-active,
  .fade-enter-active,
  .fade-leave-active {
    transition: none !important;
  }
}

/* Mobile Responsiveness (576px and below) */
@media (max-width: 576px) {
  .container-fluid {
    padding: 0 1rem;
  }
  
  .brand-text {
    font-size: 1.1rem;
  }
  
  .brand-icon {
    font-size: 1.5rem;
  }
}

/* Tablet adjustments (768px and below) */
@media (max-width: 768px) {
  .user-role {
    display: none;
  }
}
</style>