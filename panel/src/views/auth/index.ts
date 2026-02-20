
import Auth from "./Auth.vue";
import Login from './Login.vue'
import Register from "./Register.vue";
import Reset from "./Reset.vue";
import ResetComplete from "./ResetComplete.vue";
import VerifyEmail from "./VerifyEmail.vue";
import VerificationRequired from "./VerificationRequired.vue";
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
      {
        path: '/auth/verify-required',
        name: 'verifyRequired',
        component: VerificationRequired
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

