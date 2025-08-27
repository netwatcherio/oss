import {createApp} from 'vue'
// @ts-ignore
import App from './App.vue'
import router from './router'

const app = createApp(App)

app.config.warnHandler = function (msg, vm, trace) {
    console.log(`Warn: ${msg}\nTrace: ${trace}`);
}

app.config.errorHandler = function (msg, vm, trace) {
    console.error(`Error: ${msg}\nTrace: ${trace}`);
}

app.use(router)

app.mount('#app')
