/** Sticky notes for the loaded snapshot: per-anchor counts and the threads that are open. */

import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

import { api } from '../api/client'
import { anchorKey, anchorTableId } from '../notes/anchors'
import { useWorkspace } from './workspace'
import type { Anchor } from '../notes/anchors'
import type { AnchorKind, Note } from '../api/types'

export type Count = { open: number; resolved: number }

const ZERO: Count = { open: 0, resolved: 0 }

export const useNotes = defineStore('notes', () => {
  const workspace = useWorkspace()

  const counts = ref(new Map<string, Count>())
  // Relationship anchor key → declaring table, from the counts.
  const relTable = ref(new Map<string, string>())
  const threads = ref(new Map<string, Note[]>())

  // Bumped on snapshot change; a response started under an older one is dropped.
  let generation = 0

  function countFor(a: Anchor): Count {
    return counts.value.get(anchorKey(a)) ?? ZERO
  }

  const openTablesWithNotes = computed(() => {
    const out = new Set<string>()
    for (const [key, c] of counts.value) {
      if (c.open === 0) continue
      const sep = key.indexOf(':')
      const a: Anchor = { kind: key.slice(0, sep) as AnchorKind, id: key.slice(sep + 1) }
      const table = a.kind === 'relationship' ? relTable.value.get(key) : anchorTableId(a, workspace.tableById)
      if (table) out.add(table)
    }
    return out
  })

  async function loadCounts() {
    const sid = workspace.snapshot?.id
    if (!sid) return
    const started = generation
    const res = await api.noteCounts(sid)
    if (started !== generation) return
    const next = new Map<string, Count>()
    const rels = new Map<string, string>()
    for (const c of res.counts) {
      const key = anchorKey({ kind: c.anchorKind, id: c.anchorId })
      next.set(key, { open: c.open, resolved: c.resolved })
      if (c.tableId) rels.set(key, c.tableId)
    }
    counts.value = next
    relTable.value = rels
  }

  async function loadThread(a: Anchor) {
    const sid = workspace.snapshot?.id
    if (!sid) return
    const started = generation
    const res = await api.listNotes(sid, a.kind, a.id)
    if (started !== generation) return
    threads.value.set(anchorKey(a), res.notes)
  }

  // Runs after a write that succeeded; a failed reload only leaves the view stale until the next open.
  async function refresh(a: Anchor) {
    await Promise.all([loadThread(a), loadCounts()]).catch(() => {})
  }

  // The loaded thread holding this note, top-level or reply.
  function anchorOf(id: string): Anchor | null {
    for (const notes of threads.value.values()) {
      for (const n of notes) {
        if (n.id === id || n.replies?.some((r) => r.id === id)) return { kind: n.anchorKind, id: n.anchorId }
      }
    }
    return null
  }

  async function add(a: Anchor, body: string, parentId?: string) {
    const sid = workspace.snapshot?.id
    if (!sid) return
    await api.createNote(sid, { anchorKind: a.kind, anchorId: a.id, body, ...(parentId ? { parentId } : {}) })
    await refresh(a)
  }

  async function editBody(id: string, body: string) {
    const n = await api.updateNote(id, { body })
    await refresh({ kind: n.anchorKind, id: n.anchorId })
  }

  async function setResolved(id: string, resolved: boolean) {
    const n = await api.updateNote(id, { resolved })
    await refresh({ kind: n.anchorKind, id: n.anchorId })
  }

  async function remove(id: string) {
    const a = anchorOf(id)
    await api.deleteNote(id)
    if (a) await refresh(a)
    else await loadCounts().catch(() => {})
  }

  watch(
    () => workspace.snapshot?.id ?? null,
    (id) => {
      generation += 1
      counts.value = new Map()
      relTable.value = new Map()
      threads.value = new Map()
      // Counts are decoration; a failed load leaves them at zero.
      if (id) loadCounts().catch(() => {})
    },
    { immediate: true },
  )

  return {
    counts,
    threads,
    openTablesWithNotes,
    countFor,
    loadCounts,
    loadThread,
    add,
    editBody,
    setResolved,
    remove,
  }
})
