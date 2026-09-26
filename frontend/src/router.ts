import { createRouter, createWebHistory, type Router, type RouterHistory } from 'vue-router'

import { ApiError, setOnForbidden, setOnUnauthorized } from './api/client'
import { Perm } from './auth/permissions'
import { useAuth } from './stores/auth'
import { useWorkspace } from './stores/workspace'
import AdminUsersView from './views/AdminUsersView.vue'
import DiffView from './views/DiffView.vue'
import HomeView from './views/HomeView.vue'
import LoginView from './views/LoginView.vue'
import ProjectView from './views/ProjectView.vue'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
    /** Without it the user is sent home with a banner. */
    permission?: Perm
  }
}

export function createAppRouter(history: RouterHistory = createWebHistory()) {
  return createRouter({
    history,
    routes: [
      { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
      { path: '/', name: 'home', component: HomeView },
      { path: '/projects/:project', name: 'project', component: ProjectView },
      { path: '/projects/:project/versions/:version', name: 'version', component: ProjectView },
      { path: '/projects/:project/diff', name: 'diff', component: DiffView },
      {
        path: '/admin/users',
        name: 'admin-users',
        component: AdminUsersView,
        meta: { permission: Perm.UserManage },
      },
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

  setOnForbidden(() => {
    useAuth().load().catch(() => undefined)
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
    if (to.meta.public) return true
    if (!auth.signedIn) return { name: 'login', query: { next: to.fullPath } }
    if (to.meta.permission && !auth.can(to.meta.permission)) {
      useWorkspace().setError(new ApiError('No access to that page.', 403, 'access.denied'))
      return { name: 'home' }
    }
    return true
  })
}
