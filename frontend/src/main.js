import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import { getToken } from './api.js'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: () => import('./views/Dashboard.vue') },
    { path: '/containers', component: () => import('./views/Containers.vue') },
    { path: '/projects', component: () => import('./views/Projects.vue') },
    { path: '/cicd', component: () => import('./views/Cicd.vue') },
    { path: '/services', component: () => import('./views/Services.vue') },
    { path: '/logs', component: () => import('./views/Logs.vue') },
    { path: '/alerts', component: () => import('./views/Alerts.vue') },
    { path: '/users', component: () => import('./views/Users.vue') },
    { path: '/ai', component: () => import('./views/Ai.vue') },
    { path: '/login', component: () => import('./views/Login.vue') }
  ]
})

router.beforeEach((to) => {
  if (to.path !== '/login' && !getToken()) {
    return '/login'
  }
})

createApp(App).use(router).mount('#app')
