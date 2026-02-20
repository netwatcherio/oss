
import Auth from "./Auth.vue";
import Login from './Login.vue'
import Register from "./Register.vue";
import Reset from "./Reset.vue";
import ResetComplete from "./ResetComplete.vue";
import VerifyEmail from "./VerifyEmail.vue";
import InviteComplete from "./InviteComplete.vue";

export default [
  {
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
      {
        path: '/auth/reset/:token',
        name: 'resetComplete',
        component: ResetComplete
      },
      {
        path: '/auth/verify-email/:token',
        name: 'verifyEmail',
        component: VerifyEmail
      },
    ]
  },
  // Invite completion route (outside Auth layout for cleaner standalone page)
  {
    path: '/invite/:token',
    name: 'inviteComplete',
    component: InviteComplete
  }
]

