import { createRouter, createWebHistory } from 'vue-router'

const routes = [
    {
        path: '/',
        name: 'home',
        component: () => import('../views/HomeView.vue'),
        meta: { solidNav: false },
    },
    {
        path: '/demo',
        name: 'demo',
        component: () => import('../views/DemoView.vue'),
        meta: { solidNav: true },
    },
    {
        path: '/privacy',
        name: 'privacy',
        component: () => import('../views/PrivacyView.vue'),
        meta: { solidNav: true },
    },
    {
        path: '/terms',
        name: 'terms',
        component: () => import('../views/TermsView.vue'),
        meta: { solidNav: true },
    },
]

const router = createRouter({
    history: createWebHistory(),
    routes,
    scrollBehavior(to, from, savedPosition) {
        if (savedPosition) {
            return savedPosition
        }
        if (to.hash) {
            return {
                el: to.hash,
                behavior: 'smooth',
            }
        }
        return { top: 0 }
    },
})

export default router
