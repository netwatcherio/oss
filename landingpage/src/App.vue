<template>
    <NavBar :always-scrolled="route.meta.solidNav" />
    <router-view />
    <SiteFooter />
</template>

<script setup>
import { useRoute } from 'vue-router'
import NavBar from './components/NavBar.vue'
import SiteFooter from './components/SiteFooter.vue'

const route = useRoute()
</script>
