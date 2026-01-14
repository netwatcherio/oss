<script lang="ts" setup>

import NavHeader from "@/components/NavHeader.vue";
import Footer from "@/components/Footer.vue";
import {onMounted, reactive} from "vue";
import core from "@/core";

const session = core.session()
const router = core.router()

const state = reactive({
  loaded: false,
})

onMounted(() => {
  if(session == null || session.token === "") {
    router.push("/auth/login")
    return
  }
  state.loaded = true
})

</script>

<template>
  <div v-if="state.loaded" class="root-container">
    <NavHeader></NavHeader>
    <main class="main-content">
      <RouterView></RouterView>
    </main>
    <Footer></Footer>
  </div>
</template>

<style scoped>
.root-container {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  background-color: var(--bs-body-bg);
}

.main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
}
</style>