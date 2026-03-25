<template>
    <nav class="navbar" :class="{ scrolled: isScrolled || alwaysScrolled }">
        <div class="container">
            <router-link to="/" class="logo">
                <div class="logo-pulse">
                    <i class="bi bi-eye"></i>
                </div>
                <span>netwatcher.io</span>
            </router-link>
            <div class="nav-links" :class="{ 'mobile-open': mobileOpen }">
                <router-link to="/#features" class="nav-link"><span>Features</span></router-link>
                <router-link to="/#comparison" class="nav-link"><span>Compare</span></router-link>
                <router-link to="/#roadmap" class="nav-link"><span>Roadmap</span></router-link>
                <router-link to="/demo" class="nav-link"><span>Demo</span></router-link>
                <a href="https://github.com/netwatcherio/oss" class="btn btn-outline btn-sm" target="_blank">
                    <i class="bi bi-github"></i> GitHub
                </a>
                <a href="https://app.netwatcher.io" class="btn btn-primary btn-sm" target="_blank">Get Started</a>
            </div>
            <button class="mobile-menu-btn" aria-label="Menu" @click="toggleMobile">
                <i class="bi" :class="mobileOpen ? 'bi-x-lg' : 'bi-list'"></i>
            </button>
        </div>
    </nav>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'

const props = defineProps({
    alwaysScrolled: {
        type: Boolean,
        default: false,
    },
})

const isScrolled = ref(false)
const mobileOpen = ref(false)
const router = useRouter()

function handleScroll() {
    isScrolled.value = window.scrollY > 50
}

function toggleMobile() {
    mobileOpen.value = !mobileOpen.value
}

// Close mobile menu on route change
router.afterEach(() => {
    mobileOpen.value = false
})

onMounted(() => {
    window.addEventListener('scroll', handleScroll)
    handleScroll()
})

onUnmounted(() => {
    window.removeEventListener('scroll', handleScroll)
})
</script>
