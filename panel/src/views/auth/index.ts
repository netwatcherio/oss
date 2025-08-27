
import Auth from "./Auth.vue";
import Login from './Login.vue'
import Register from "./Register.vue";
import Reset from "./Reset.vue";

export default {
  path: '/auth',
  name: 'auth',
  component: Auth,
  children: [
    {
      path: '/auth/login',
      name: 'login',
      component: Login
    },
    {
      path: '/auth/register',
      name: 'register',
      component: Register
    },
    {
      path: '/auth/reset',
      name: 'reset',
      component: Reset
    },
  ]
}
