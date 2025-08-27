<script lang="ts" setup>

import NavHeader from "@/components/NavHeader.vue";
import NavSidebar from "@/components/NavSidebar.vue";
import Footer from "@/components/Footer.vue";
import {onMounted, reactive} from "vue";
import core from "@/core";
import {User} from "@/types";
import Title from "@/components/Title.vue";

const session = core.session()

const state = reactive({
  loaded: false,
})


</script>

<template>
  <Title title="Profile"></Title>
  <div class="container-fluid">
    <div class="row">
      <div class="col-lg-4 col-xlg-3 col-md-5">
        <div class="card">
          <div class="card-body">
            <div class="mt-4 d-flex justify-content-center align-items-center flex-column">
              <img
                  alt="user"
                  class="profile-pic rounded-circle"
                  src="/assets/images/profile-placeholder.png"
                  width="72"
              />
              <h4 class="mt-2 text-capitalize">{{session.user.first_name}} {{session.user.last_name}}</h4>
              <h6 class="card-subtitle">{{session.user.email}}</h6>
              <div class="d-flex justify-content-center">
                <div class="">
                  {{session.user?.sites || 0}} sites
                </div>

              </div>
            </div>
          </div>
        </div>
      </div>
      <!-- Column -->
      <!-- Column -->
      <div class="col-lg-8 col-xlg-9 col-md-7">
        <div class="card overflow-hidden">
          <!-- Tabs -->
          <ul class="nav nav-pills" id="pills-tab" role="tablist" sty>
            <li class="nav-item">
              <a
                  class="nav-link active"
                  id="profile-tab"
                  data-bs-toggle="pill"
                  href="#profile"
                  role="tab"
                  aria-controls="pills-profile"
                  aria-selected="true"
              >Profile</a
              >
            </li>
            <li class="nav-item">
              <a
                  class="nav-link"
                  id="pills-modify"
                  data-bs-toggle="pill"
                  href="#modify"
                  role="tab"
                  aria-controls="pills-modify"
                  aria-selected="false"
              >Modify</a
              >
            </li>
          </ul>
          <!-- Tabs -->
          <div class="tab-content" id="pills-tabContent">
            <div
                class="tab-pane fade show active"
                id="profile"
                role="tabpanel"
                aria-labelledby="pills-profile-tab"
            >
              <div class="card-body">
                <div class="d-flex">
                  <div class="col-md-3 col-xs-6 b-r">
                    <strong>Full Name</strong>
                    <br />
                    <p class="text-muted text-capitalize">{{session.user.first_name}} {{session.user.last_name}}</p>
                  </div>

                  <div class="col-md-3 col-xs-6 b-r">
                    <strong>Email</strong>
                    <br />
                    <p class="text-muted">{{session.user.email}}</p>
                  </div>

                  <div class="col-md-3 col-xs-6 b-r">
                    <strong>Verification</strong>
                    <br />
                    <p v-if="session.user.verified" class="text-muted"><i class="ri-checkbox-circle-line"></i> Verified</p>
                    <p v-else class="text-danger">Not Verified</p>
                  </div>

                  <div class="col-md-3 col-xs-6 b-r">
                    <strong>Account</strong>
                    <br />
                    <p v-if="session.user.admin" class="text-muted">Admin</p>
                    <p v-else class="text-muted">Default</p>
                  </div>

                </div>

                <h4 class="font-weight-medium mt-4">Usage</h4>
                <hr />
                <h5 class="mt-4">
                  Sites <span class="pull-right">1/10</span>
                </h5>
                <div class="progress">
                  <div
                      class="progress-bar bg-success"
                      role="progressbar"
                      aria-valuenow="80"
                      aria-valuemin="0"
                      aria-valuemax="100"
                      style="width: 10%; height: 6px"
                  >
                    <span class="sr-only">50% Complete</span>
                  </div>
                </div>

              </div>
            </div>
            <div
                class="tab-pane fade"
                id="modify"
                role="tabpanel"
                aria-labelledby="pills-setting-tab">
              <div class="card-body">
                <form class="form-horizontal form-material">
                  <div class="mb-3">
                    <label class="col-md-12">Full Name</label>
                    <div class="col-md-12">
                      <input
                          type="text"
                          placeholder="Johnathan Doe"
                          class="form-control form-control-line"
                      />
                    </div>
                  </div>
                  <div class="mb-3">
                    <label for="example-email" class="col-md-12"
                    >Email</label
                    >
                    <div class="col-md-12">
                      <input
                          type="email"
                          placeholder="johnathan@admin.com"
                          class="form-control form-control-line"
                          name="example-email"
                          id="example-email"
                      />
                    </div>
                  </div>
                  <div class="mb-3">
                    <label class="col-md-12">Password</label>
                    <div class="col-md-12">
                      <input
                          type="password"
                          value="password"
                          class="form-control form-control-line"
                      />
                    </div>
                  </div>
                  <div class="mb-3">
                    <label class="col-md-12">Phone No</label>
                    <div class="col-md-12">
                      <input
                          type="text"
                          placeholder="123 456 7890"
                          class="form-control form-control-line"
                      />
                    </div>
                  </div>
                  <div class="mb-3">
                    <label class="col-md-12">Message</label>
                    <div class="col-md-12">
                            <textarea
                                rows="5"
                                class="form-control form-control-line"
                            ></textarea>
                    </div>
                  </div>
                  <div class="mb-3">
                    <label class="col-sm-12">Select Country</label>
                    <div class="col-sm-12">
                      <select class="form-control form-control-line">
                        <option>London</option>
                        <option>India</option>
                        <option>Usa</option>
                        <option>Canada</option>
                        <option>Thailand</option>
                      </select>
                    </div>
                  </div>
                  <div class="mb-3">
                    <div class="col-sm-12">
                      <button class="btn btn-success">
                        Update Profile
                      </button>
                    </div>
                  </div>
                </form>
              </div>
            </div>
          </div>
        </div>
      </div>
      <!-- Column -->
    </div>
    <!-- Row -->
    <!-- ============================================================= -->
    <!-- End PAge Content -->
    <!-- ============================================================= -->
  </div>
</template>

<style>

</style>