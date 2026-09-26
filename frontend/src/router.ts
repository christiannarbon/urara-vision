import { createRouter, createWebHistory, type Router, type RouterHistory } from 'vue-router'

import { setOnUnauthorized } from './api/client'
import { useAuth } from './stores/auth'
import DiffView from './views/DiffView.vue'
import HomeView from './views/HomeView.vue'
import LoginView from './views/LoginView.vue'
import ProjectView from './views/ProjectView.vue'

export function createAppRouter(history: RouterHistory = createWebHistory()) {
  return createRouter({
    history,
    routes: [
      { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
      { path: '/', name: 'home', component: HomeView },
      { path: '/projects/:project', name: 'project', component: ProjectView },
      { path: '/projects/:project/versions/:version', name: 'version', component: ProjectView },
      { path: '/projects/:project/diff', name: 'diff', component: DiffView },
      { path: '/:pathMatch(.*)*', redirect: '/' },
    ],
  })
}

/** A same-site path, or `/`. `//host` and `/\host` are other sites to a browser. */
export function safeNext(next: unknown): string {
  if (typeof next !== 'string' || !next.startsWith('/') || next.startsWith('//') || next.startsWith('/\\')) {
    return '/'
  }
  return next
}

/**
 * Sign-in guard. Separate from createAppRouter so view tests with a mocked API
 * can build a router without a `/me` call.
 */
export function installAuthGuard(router: Router) {
  setOnUnauthorized(() => {
    const auth = useAuth()
    const wasSignedIn = auth.signedIn
    auth.clear()
    const current = router.currentRoute.value
    if (wasSignedIn && !current.meta.public) {
      void router.push({ name: 'login', query: { next: current.fullPath } })
    }
  })

  router.beforeEach(async (to) => {
    const auth = useAuth()
    if (!auth.loaded) {
      try {
        await auth.load()
      } catch {
        // Unreachable backend: the login page will say so on submit.
      }
    }
    if (to.name === 'login') return auth.signedIn ? safeNext(to.query.next) : true
    if (to.meta.public || auth.signedIn) return true
    return { name: 'login', query: { next: to.fullPath } }
  })
}
