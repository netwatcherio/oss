<script lang="ts" setup>
import {useRouter} from "vue-router";
import {reactive} from "vue";
import {User} from "@/types";
import authService from "@/services/authService";
import core from "@/core";
import Loader from "@/components/Loader.vue";

const router = useRouter()


const state = reactive({
  user: {} as User,
  confirmPassword: "",
  errorMessage: "",
  waiting: false,
  began: 0,
  error: false,
})

interface Response {
  token: string
  user: User
}

let session = core.session()

function onRegister(response: any) {
  done()
  /*  let data = response.data as Response
    session.token = data.token
    session.data = data.user*/
  // TODO log successful login and let user know they successfully registered / prompt for email verification
  // console.log(response)
  router.push("/auth/login")
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

function onFailure(error: any) {
  done()
  state.error = true
  console.log(error)
  switch (error.response.data) {
    case "user exists":
      state.errorMessage = "The email provided has already been registered for an account."
      break;
    case "invalid":
      state.errorMessage = "Invalid request. Please check your information."
      break;
    default:
      state.errorMessage = "An error occurred. Please try again later."
      break;
  }

}

function submit(_: MouseEvent) {
  begin()
  authService.authRegister(state.user).then(onRegister).catch(onFailure)
}


</script>

<template>
  <div class="d-flex h-100">
    <div class="col-lg-3 col-xl-3 bg-primary on-sidebar" style="height: 100vh;">
      <div class="h-100 d-flex align-items-center justify-content-center">
        <div class="row justify-content-center text-center">
          <div class="col-md-7 col-lg-12 col-xl-9">
            <div>
              <router-link to="/" class="navbar-brand">
        <i class="fa-solid fa-eye brand-icon"></i>
        <span class="brand-text">netwatcher.io</span>
      </router-link>
            </div>
            <!--<h2 class="text-white mt-4 fw-light">
              <span class="font-weight-medium">Network Monitoring</span> made easy
            </h2>-->
            <p class="op-5 text-white fs-4 mt-4">
              A simple network performance monitoring platform designed for MSPs
            </p>
          </div>
        </div>
      </div>
    </div>
    <div class="
            col-lg-8
            d-flex
            align-items-center
            justify-content-center
          ">
      <div class="col-5">
        <div class="card">
          <div class="card-body d-flex flex-column gap-1">
            <div class="d-flex align-items-end justify-content-between gap-1">
              <div class="d-flex gap-2">
                <div class="label-title">Register</div>
              </div>
              <span class="label-subtext mb-1">
              Have an account?
              <router-link id="to-register" to="/auth/login">login</router-link>
            </span>
            </div>
            <div class="form-horizontal needs-validation mt-2">
              <div class="form-floating mb-2">
                <input id="nw-firstname" v-model="state.user.firstName" class="form-control form-input-bg"
                       name="first_name" placeholder="john deo"
                       required="" type="text">
                <label for="nw-firstname">First Name</label>
                <div class="invalid-feedback">First name is required</div>
              </div>
              <div class="form-floating mb-2">
                <input id="nw-lastname" v-model="state.user.lastName" class="form-control form-input-bg"
                       name="last_name" placeholder="john deo"
                       required="" type="text">
                <label for="nw-lastname">Last Name</label>
                <div class="invalid-feedback">Last name is required</div>
              </div>
              <div class="form-floating mb-2">
                <input id="nw-remail" v-model="state.user.email" class="form-control form-input-bg" name="email"
                       placeholder="john@gmail.com"
                       required="" type="email">
                <label for="nw-remail">Email</label>
                <div class="invalid-feedback">Email is required</div>
              </div>
              <div class="form-floating mb-2">
                <input id="text-rpassword" v-model="state.user.password" class="form-control form-input-bg"
                       name="password" placeholder="*****"
                       required="" type="password">
                <label for="text-rpassword">Password</label>
                <div class="invalid-feedback">Password is required</div>
              </div>
              <div class="form-floating mb-2">
                <input id="nw-rcpassword" v-model="state.confirmPassword" class="form-control form-input-bg"
                       name="password_confirm" placeholder="*****"
                       required="" type="password">
                <label for="nw-rcpassword">Confirm Password</label>
                <div class="invalid-feedback">Password is required</div>
              </div>
              <div class="d-flex justify-content-between align-items-center">
                <div class="form-check mx-1">
                  <input id="r-me" class="form-check-input" required="" type="checkbox" value="">
                  <label class="form-check-label" for="r-me">
                    Remember me
                  </label>
                </div>
                <div class="d-flex align-items-center gap-3">
                <div class="d-flex align-items-center justify-content-center">
                  <Loader v-if="state.waiting" inverse large></Loader>
                </div>
                <button class="btn btn-primary btn-lg px-4" type="submit" @click="submit" :disabled="state.waiting">
                  register
                </button>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div class="" style="height: 5rem">
          <div class="mt-2 " v-if="state.error"
               :class="`${state.waiting?'error-message-pending':'error-message-animation message-body '}`">
            <div class="text-danger" v-if="!state.waiting">{{ state.errorMessage }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Logo/Brand */
.navbar-brand {
  align-items: center;
  /*gap: 0.75rem;*/
  text-decoration: none;
  color: #FFFF;
  font-weight: 600;
  font-size: 1.25rem;
  transition: all 0.2s;
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