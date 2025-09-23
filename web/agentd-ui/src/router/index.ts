import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'overview',
      component: () => import('@/views/OverviewView.vue')
    },
    {
      path: '/specialists',
      name: 'specialists',
      component: () => import('@/views/SpecialistsView.vue')
    },
    {
      path: '/chat',
      name: 'chat',
      component: () => import('@/views/ChatView.vue')
    },
    {
      path: '/flow',
      name: 'flow',
      component: () => import('@/views/FlowView.vue')
    },
    {
      path: '/runs',
      name: 'runs',
      component: () => import('@/views/RunsView.vue')
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue')
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue')
    }
  ]
})

export default router
