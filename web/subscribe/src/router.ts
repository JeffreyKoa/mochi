import { createRouter, createWebHistory } from 'vue-router'
import { isLoggedIn } from './api'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/catalog' },
    {
      path: '/login',
      name: 'login',
      component: () => import('./views/LoginView.vue'),
      meta: { guest: true },
    },
    {
      path: '/catalog',
      name: 'catalog',
      component: () => import('./views/CatalogView.vue'),
      meta: { auth: true },
    },
    {
      path: '/success',
      name: 'success',
      component: () => import('./views/SuccessView.vue'),
      meta: { auth: true },
    },
  ],
})

router.beforeEach((to) => {
  const loggedIn = isLoggedIn()
  if (to.meta.auth && !loggedIn) return { name: 'login' }
  if (to.meta.guest && loggedIn) return { name: 'catalog' }
  return true
})

export default router
