import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import { getToken } from './api.js'
import Dashboard from './views/Dashboard.vue'
import Containers from './views/Containers.vue'
import Projects from './views/Projects.vue'
import Cicd from './views/Cicd.vue'
import Services from './views/Services.vue'
import Logs from './views/Logs.vue'
import Alerts from './views/Alerts.vue'
import Login from './views/Login.vue'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Dashboard },
    { path: '/containers', component: Containers },
    { path: '/projects', component: Projects },
    { path: '/cicd', component: Cicd },
    { path: '/services', component: Services },
    { path: '/logs', component: Logs },
    { path: '/alerts', component: Alerts },
    { path: '/login', component: Login }
  ]
})

router.beforeEach((to) => {
  if (to.path !== '/login' && !getToken()) {
    return '/login'
  }
})

createApp(App).use(router).mount('#app')
