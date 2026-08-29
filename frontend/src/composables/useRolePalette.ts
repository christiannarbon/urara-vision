/**
 * The theme's two role hues, read reactively.
 *
 * Role colours are derived from --fact and --dim rather than defined per role
 * (see graph/roles.ts for why), and those two tokens change with the theme.
 * Cytoscape and plain DOM both need the resolved values: the canvas cannot
 * resolve a custom property at all, and a panel could use var(--fact) but not
 * the shifted hue built from it.
 */

import { computed, onMounted, ref, watch } from 'vue'

import { roleColor, roleSpec, type RoleSpec } from '../graph/roles'
import { useTheme } from './useTheme'

/** Reads the resolved value of a CSS custom property. */
function token(name: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

export function useRolePalette() {
  const { art } = useTheme()
  const fact = ref('#0f766e')
  const dim = ref('#b45309')

  function read() {
    fact.value = token('--fact', '#0f766e')
    dim.value = token('--dim', '#b45309')
  }

  onMounted(read)
  // A frame's wait: switching theme sets [data-art] on the document element,
  // and the new custom-property values are not committed until after that.
  watch(art, () => requestAnimationFrame(read))

  /** The colour for a role id, whether or not it is one of the built-in roles. */
  const colorOf = computed(() => (id: string) => roleColor(roleSpec(id), fact.value, dim.value))

  /** The colour for a spec already in hand. */
  const colorOfSpec = computed(() => (spec: RoleSpec) => roleColor(spec, fact.value, dim.value))

  return { fact, dim, colorOf, colorOfSpec }
}
