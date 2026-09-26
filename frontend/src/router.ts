import { createRouter, createWebHistory, type RouterHistory } from 'vue-router'

import DiffView from './views/DiffView.vue'
import HomeView from './views/HomeView.vue'
import ProjectView from './views/ProjectView.vue'

export function createAppRouter(history: RouterHistory = createWebHistory()) {
  return createRouter({
    history,
    routes: [
      { path: '/', name: 'home', component: HomeView },
      { path: '/projects/:project', name: 'project', component: ProjectView },
      { path: '/projects/:project/versions/:version', name: 'version', component: ProjectView },
      { path: '/projects/:project/diff', name: 'diff', component: DiffView },
      { path: '/:pathMatch(.*)*', redirect: '/' },
    ],
  })
}
