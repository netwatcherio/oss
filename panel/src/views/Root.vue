<script lang="ts" setup>

import NavHeader from "@/components/NavHeader.vue";
import Footer from "@/components/Footer.vue";
import {onMounted, reactive} from "vue";
import core from "@/core";
import { AuthService } from "@/services/apiService";

const session = core.session()
const router = core.router()

const state = reactive({
  loaded: false,
})

onMounted(async () => {
  if(session == null || session.token === "") {
    router.push("/auth/login")
    return
  }

  // Check if unverified user needs to verify
  try {
    const [me, config] = await Promise.all([
      AuthService.getMe(),
      AuthService.getConfig()
    ]);
    if (config.email_verification_required && !me.verified) {
      router.push("/auth/verify-required");
      return;
    }
  } catch {
    // If check fails, proceed (fail-open for self-hosted setups)
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