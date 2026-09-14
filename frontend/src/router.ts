import { createRouter, createWebHistory, type RouterHistory } from 'vue-router'

import HomeView from './views/HomeView.vue'
import ProjectView from './views/ProjectView.vue'

export function createAppRouter(history: RouterHistory = createWebHistory()) {
  return createRouter({
    history,
    routes: [
      { path: '/', name: 'home', component: HomeView },
      { path: '/projects/:project', name: 'project', component: ProjectView },
      { path: '/:pathMatch(.*)*', redirect: '/' },
    ],
  })
}
