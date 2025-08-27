<script lang="ts" setup>
import {reactive} from "vue";
import authService from "@/services/authService";
import type {User} from "@/types"
import core from "@/core";
import Loader from "@/components/Loader.vue";
import FormElement from "@/components/FormElement.vue";

const state = reactive({
  user: {} as User,
  began: 0 as number,
  waiting: false,
  error: false
})

const router = core.router();

interface Response {
  token: string
  data: User
}

let session = core.session()

function onLogin(response: any) {
  done()
  let data = response.data as Response
  session.token = data.token
  session.data = data.data
  router.push("/")
}

function onFailure(error: any) {

  done()
  state.error = true
  console.log(error)
}

function begin() {
  state.waiting = true
  state.began = Date.now().valueOf()
}

function done() {
  if (state.waiting) {
    let delta = Date.now().valueOf() - state.began
    let minTimeout = 250
    setTimeout(() => {
      state.waiting = false
      state.began = 0
    }, Math.max(minTimeout - delta, 0))
  }
}

function submit(_: MouseEvent) {
  begin()
  // Attempt to log in with the provided credentials
  authService.authLogin(state.user).then(onLogin).catch(onFailure)
}

</script>

<template>
  <div class="d-flex justify-content-center align-items-center" style="height: 75vh">
    <div class="form-entry w-100">
      <FormElement title="Login">
        <template #alternate>
        <span class="label-subtext mb-1">
                Need an account?
                <router-link id="to-register" to="/auth/register">register</router-link>
              </span>
        </template>
        <template #body>
          <div class="form-horizontal needs-validation mt-2">
            <div class="form-floating mb-2">
              <input id="tb-email" v-model="state.user.email" class="form-control form-input-bg" name="email"
                     placeholder="name@example.com" required="" type="email">
              <label for="tb-email">Email</label>
              <div class="invalid-feedback">email is required</div>
            </div>

            <div class="form-floating mb-2">
              <input id="current-password" v-model="state.user.password" class="form-control form-input-bg"
                     name="password"
                     placeholder="*****" required="" type="password">
              <label for="current-password">Password</label>
              <div class="invalid-feedback">password is required</div>
            </div>

            <div class="d-flex align-items-center justify-content-between">
              <div>
                <router-link id="to-recover" class="label-c4 label-w600 label-underlined px-1" to="/auth/reset">forgot
                  password?
                </router-link>
              </div>
              <div class="d-flex align-items-center gap-3">
                <div class="d-flex align-items-center justify-content-center">
                  <Loader v-if="state.waiting" inverse large></Loader>
                </div>
                <button class="btn btn-primary btn-lg px-4 d-flex align-items-center gap-1" @click="submit"
                        :disabled="state.waiting">

                  login
                </button>
              </div>
            </div>

          </div>
        </template>
      </FormElement>
      <div class="" style="height: 4rem">
        <div class="mt-2 " v-if="state.error"
             :class="`${state.waiting?'error-message-pending':'error-message-animation message-body '}`">
          <div class="text-danger" v-if="!state.waiting">Incorrect Email/Password combination. Please try again.</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.form-entry {
  max-width: 26rem;
  width: 100%;
}
.card {
  border-radius: 0.8rem;
}

.card-body {

  padding: 0.8rem;
}

.error-message-pending {
  animation: animate-pending 100ms forwards ease-out;

}

@keyframes animate-pending {
  0% {
    filter: saturate(90%) blur(1px);
    height: 100%;
  }

  100% {
    filter: saturate(50%) blur(8px);
    opacity: 0;
    height: 0%;
  }
}

.message-body {

  margin-bottom: 0.375rem;
  background-color: rgba(255, 64, 64, 0.3);
  border: 1px solid rgba(255, 64, 64, 1);
  width: 100%;
  padding: 0.5rem 1rem;
  display: flex;
  align-items: center;
  border-radius: 0.8rem;
}

.error-message-animation {
  animation: animate-expand-vertical 250ms forwards ease-out;
}

@keyframes animate-expand-vertical {
  0% {
    scale: 0.95;
    opacity: 0.8;
    filter: blur(2px);
  }
  50% {

  }
  100% {
    scale: 1;
    opacity: 1;
    filter: blur(0px);
  }
}

.btn-primary.btn-lg {
  border-radius: 0.375rem !important;

}


</style>