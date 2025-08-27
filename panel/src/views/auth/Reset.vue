<script lang="ts" setup>
import {reactive} from "vue";
import authService from "@/services/authService";
import profileService from "@/services/profile";
import type {User} from "@/types"
import core from "@/core";
import Loader from "@/components/Loader.vue";

const state = reactive({
  user: {} as User,
  waiting: false,
  error: false
})

interface Response {
  token: string
}

let session = core.session()
let router = core.router()

function onLogin(response: any) {
  state.waiting = false
  let data = response.data as Response
  session.token = data.token
  profileService.getProfile().then((res) => {
    session.user = res.data as User
  }).catch((err) => {
    console.log(err)
  })
  router.push("/")
}

function onFailure(error: any) {
  state.waiting = false
  state.error = true
  console.log(error)
}

function submit(_: MouseEvent) {
  state.waiting = true
  // Attempt to log in with the provided credentials
  authService.login(state.user).then(onLogin).catch(onFailure)
}

</script>

<template>
  <div class="row auth-wrapper gx-0">
    <div class="col-lg-4 col-xl-3 bg-primary auth-box-2 on-sidebar">
      <div class="h-100 d-flex align-items-center justify-content-center">
        <div class="row justify-content-center text-center">
          <div class="col-md-7 col-lg-12 col-xl-9">
            <div>
              <span class="db"><img alt="logo" src="/assets/images/logo-light-icon.png"></span>
              <span class="db"><img alt="logo" src="/assets/images/logo-light-text.png"></span>
            </div>
            <h2 class="text-white mt-4 fw-light">
              <span class="font-weight-medium">Network Monitoring</span> made easy
            </h2>
            <p class="op-5 text-white fs-4 mt-4">
              A simple network performance monitoring platform designed for MSPs
            </p>
          </div>
        </div>
      </div>
    </div>
    <div class="
            col-lg-8 col-xl-9
            d-flex
            align-items-center
            justify-content-center
          ">
      <div class="row justify-content-center w-100 mt-4 mt-lg-0">
        <div class="col-lg-6 col-xl-3 col-md-7">
          <div class="card">
            <div class="card-body">
              <h1>login</h1>
              <p class="text-muted fs-4 mb-2">
                new here?
                <router-link id="to-register" to="/auth/register">create an account</router-link>
              </p>
              <div class="text-danger" v-if="state.error">Incorrect Email/Password combination. Please try again.</div>
              <div v-else>&nbsp;</div>
              <div class="form-horizontal needs-validation mt-2">
                <div class="form-floating mb-3">
                  <input id="tb-email" v-model="state.user.email" class="form-control form-input-bg" name="email"
                         placeholder="name@example.com" required="" type="email">
                  <label for="tb-email">email</label>
                  <div class="invalid-feedback">email is required</div>
                </div>

                <div class="form-floating mb-3">
                  <input id="current-password" v-model="state.user.password" class="form-control form-input-bg"
                         name="password"
                         placeholder="*****" required="" type="password">
                  <label for="current-password">password</label>
                  <div class="invalid-feedback">password is required</div>
                </div>

                <div class="d-flex align-items-center mb-3">
                  <div class="ms-auto">
                    <router-link id="to-recover" class="fw-bold" to="/auth/reset">forgot password?</router-link>
                  </div>
                </div>
                <div class="d-flex align-items-stretch button-group mt-4 pt-2">
                  <button class="btn btn-primary btn-lg px-4" @click="submit" :disabled="state.waiting">
                    login
                    <Loader v-if="state.waiting"></Loader>
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style>

</style>