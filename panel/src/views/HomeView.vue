<script>
import {defineComponent} from "vue";
import Title from "@/components/Title.vue";

export default defineComponent({
  components: {Title}
})
</script>

<template>

  <div class="container-fluid">
  <Title title="this is a placeholder"></Title>
    <div class="row">
      <!-- column -->
<!--      <div class="col-12">
        <div class="alert alert-info" role="alert">
          todo: add some sort of overview here
        </div>
        <div class="card">
          <div class="card-body">
            &lt;!&ndash; title &ndash;&gt;
            <div class="d-md-flex">
              <div>
                <h4 class="card-title">sites</h4>
&lt;!&ndash;                <h5 class="card-subtitle">sites</h5>&ndash;&gt;
              </div>
            </div>
            &lt;!&ndash; title &ndash;&gt;
            <div class="table-responsive">
              <table class="table mb-0 table-hover align-middle text-nowrap">
                <thead>
                <tr>
                  &lt;!&ndash;TODO make custom icons for this or something to indicate wether it's online or not&ndash;&gt;
                  <th class="border-top-0">Status</th>
                  <th class="border-top-0">Name</th>
                  <th class="border-top-0">Location</th>
                  <th class="border-top-0">ISP</th>
                  <th class="border-top-0">Type</th>
                  <th class="border-top-0">Last Seen</th>
                </tr>
                </thead>
                <tbody>
                <tr>
                  <td>
                    <label class="badge bg-success">Online</label>
                  </td>
                  <td>TOPS</td>
                  <td>Kelowna, BC</td>
                  <td>Telus</td>
                  <td>MacOS - Dev</td>
                  <td>
                    <label class="badge bg-info">13s ago</label>
                  </td>
                </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>-->
    </div>
  </div>
</template>

<style>

</style>