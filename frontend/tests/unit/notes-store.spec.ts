/** The notes store. */

import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Note, NoteCount, Snapshot, TableResponse } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      noteCounts: vi.fn(),
      listNotes: vi.fn(),
      createNote: vi.fn(),
      updateNote: vi.fn(),
      deleteNote: vi.fn(),
    },
  }
})

const { api } = await import('../../src/api/client')
const { useNotes } = await import('../../src/stores/notes')
const { useWorkspace } = await import('../../src/stores/workspace')
const { columnAnchor, relationshipAnchor, tableAnchor } = await import('../../src/notes/anchors')

function snapshot(id: string): Snapshot {
  return { id, name: id, sourceLabel: 'docs', createdAt: '', stats: {} as Snapshot['stats'], projectId: 'p', projectSlug: 'p' }
}

function count(anchorKind: NoteCount['anchorKind'], anchorId: string, open: number, resolved = 0): NoteCount {
  return { anchorKind, anchorId, open, resolved }
}

function note(id: string): Note {
  return { id, snapshotId: 's1', anchorKind: 'table', anchorId: 'd/t', body: 'b', authorName: 'a', createdAt: '', updatedAt: '' }
}

function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => (resolve = res))
  return { promise, resolve }
}

const flush = () => new Promise((r) => setTimeout(r))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.noteCounts).mockResolvedValue({ counts: [] })
  vi.mocked(api.listNotes).mockResolvedValue({ notes: [] })
})

describe('useNotes', () => {
  it('loads counts when the snapshot changes', async () => {
    const workspace = useWorkspace()
    const notes = useNotes()
    vi.mocked(api.noteCounts).mockResolvedValueOnce({ counts: [count('table', 'd/t', 2, 1)] })
    workspace.snapshot = snapshot('s1')
    await nextTick()
    await flush()
    expect(api.noteCounts).toHaveBeenCalledWith('s1')
    expect(notes.countFor(tableAnchor('d/t'))).toEqual({ open: 2, resolved: 1 })
    expect(notes.countFor(tableAnchor('d/other'))).toEqual({ open: 0, resolved: 0 })
  })

  it('drops a response for the previous snapshot', async () => {
    const workspace = useWorkspace()
    const notes = useNotes()
    const slow = deferred<{ counts: NoteCount[] }>()
    vi.mocked(api.noteCounts)
      .mockReturnValueOnce(slow.promise)
      .mockResolvedValueOnce({ counts: [count('table', 'd/new', 1)] })

    workspace.snapshot = snapshot('s1')
    await nextTick()
    workspace.snapshot = snapshot('s2')
    await nextTick()
    await flush()
    slow.resolve({ counts: [count('table', 'd/old', 5)] })
    await flush()

    expect(notes.countFor(tableAnchor('d/old')).open).toBe(0)
    expect(notes.countFor(tableAnchor('d/new')).open).toBe(1)
  })

  it('add reloads the thread and the counts', async () => {
    const workspace = useWorkspace()
    workspace.snapshot = snapshot('s1')
    const notes = useNotes()
    await flush()
    vi.mocked(api.createNote).mockResolvedValue(note('n1'))
    vi.mocked(api.listNotes).mockResolvedValue({ notes: [note('n1')] })
    vi.mocked(api.noteCounts).mockResolvedValue({ counts: [count('table', 'd/t', 1)] })

    await notes.add(tableAnchor('d/t'), 'hello')

    expect(api.createNote).toHaveBeenCalledWith('s1', { anchorKind: 'table', anchorId: 'd/t', body: 'hello' })
    expect(api.listNotes).toHaveBeenCalledWith('s1', 'table', 'd/t')
    expect(notes.threads.get('table:d/t')?.map((n) => n.id)).toEqual(['n1'])
    expect(notes.countFor(tableAnchor('d/t')).open).toBe(1)
  })

  it('openTablesWithNotes counts open column notes and skips resolved ones', async () => {
    const workspace = useWorkspace()
    vi.mocked(api.noteCounts).mockResolvedValue({
      counts: [
        count(columnAnchor('d/open', 'id').kind, columnAnchor('d/open', 'id').id, 1),
        count('table', 'd/closed', 0, 2),
        count(relationshipAnchor('r1').kind, 'r1', 1),
      ],
    })
    workspace.snapshot = snapshot('s1')
    const notes = useNotes()
    await flush()

    expect([...notes.openTablesWithNotes]).toEqual(['d/open'])

    // A relationship note counts once its table detail is loaded.
    workspace.detail = {
      table: { relationships: [{ id: 'r1', fromTableId: 'd/rel' }] },
    } as unknown as TableResponse
    expect(notes.openTablesWithNotes.has('d/rel')).toBe(true)
  })
})
