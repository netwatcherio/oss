<script lang="ts" setup>

import NavHeader from "@/components/NavHeader.vue";
import NavSidebar from "@/components/NavSidebar.vue";
import Footer from "@/components/Footer.vue";
import {onMounted, reactive} from "vue";
import core from "@/core";
import {User} from "@/types";

const session = core.session()
const router = core.router()

const state = reactive({
  loaded: false,
})

onMounted(() => {
  if(session.token === "") {
    router.push("/auth/login")
    return
  }
  state.loaded = true
})

</script>

<template>
  <div v-if="state.loaded" class="container-fluid px-0 mx-0 h-100">
    <div class="d-flex h-100">
      <!--<div style="height: 100vh;">
        <NavSidebar></NavSidebar>
      </div>-->
      <div class="flex-fill d-flex flex-column" style="height: 100vh;">
        <NavHeader></NavHeader>

        <RouterView></RouterView>
        <div class="flex-fill"></div>
        <Footer></Footer>
      </div>
    </div>
  </div>
</template>

<style>

</style>