/** NotesThread, NoteItem and NotesButton over a mocked API. */

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Note, Snapshot, User } from '../../src/api/types'
import { messages as en } from '../../src/i18n/messages/en'

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
const { useAuth } = await import('../../src/stores/auth')
const { useWorkspace } = await import('../../src/stores/workspace')
const NotesThread = (await import('../../src/components/notes/NotesThread.vue')).default
const NotesButton = (await import('../../src/components/notes/NotesButton.vue')).default

const ANCHOR = { kind: 'table' as const, id: 'sales/fact_order' }
const VIEWER = ['chat.use', 'note.write', 'project.view']
const ADMIN = [...VIEWER, 'note.moderate']

function user(id: string): User {
  return { id, username: id, displayName: '', role: 'viewer', createdAt: '', updatedAt: '' }
}

function note(id: string, authorId: string, over: Partial<Note> = {}): Note {
  return {
    id,
    snapshotId: 's1',
    anchorKind: 'table',
    anchorId: ANCHOR.id,
    body: `body of ${id}`,
    authorId,
    authorName: authorId,
    createdAt: '2026-09-01T00:00:00Z',
    updatedAt: '2026-09-01T00:00:00Z',
    ...over,
  }
}

function signIn(id: string | null, permissions: string[]) {
  const auth = useAuth()
  auth.user = id ? user(id) : null
  auth.kind = 'user'
  auth.permissions = permissions
}

const mounted: VueWrapper[] = []

async function thread(notes: Note[]) {
  vi.mocked(api.listNotes).mockResolvedValue({ notes })
  const w = mount(NotesThread, { props: { anchor: ANCHOR }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return w
}

const buttons = (w: VueWrapper, text: string) => w.findAll('button').filter((b) => b.text() === text)

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.noteCounts).mockResolvedValue({ counts: [] })
  const workspace = useWorkspace()
  workspace.snapshot = { id: 's1' } as Snapshot
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
})

describe('NotesThread', () => {
  it('renders a body as text, never HTML', async () => {
    signIn('alice', VIEWER)
    const w = await thread([note('n1', 'bob', { body: '<img src=x onerror=alert(1)>' })])
    expect(w.find('.body').element.textContent).toBe('<img src=x onerror=alert(1)>')
    expect(w.find('img').exists()).toBe(false)
  })

  it('lets a viewer add, reply and resolve, and change only their own notes', async () => {
    signIn('alice', VIEWER)
    const w = await thread([note('mine', 'alice'), note('theirs', 'bob')])
    const [mine, theirs] = w.findAll('article')

    expect(buttons(w, en['notes.item.reply'])).toHaveLength(2)
    expect(buttons(w, en['notes.item.resolve'])).toHaveLength(2)
    expect(mine.text()).toContain(en['notes.item.edit'])
    expect(theirs.text()).not.toContain(en['notes.item.edit'])
    expect(theirs.text()).not.toContain(en['notes.item.delete'])

    vi.mocked(api.createNote).mockResolvedValue(note('n2', 'alice'))
    await w.find('form textarea').setValue('new note')
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(api.createNote).toHaveBeenCalledWith('s1', { anchorKind: 'table', anchorId: ANCHOR.id, body: 'new note' })

    await theirs.findAll('button').find((b) => b.text() === en['notes.item.reply'])!.trigger('click')
    await theirs.find('textarea').setValue('a reply')
    await theirs.findAll('button').filter((b) => b.text() === en['notes.item.reply']).at(-1)!.trigger('click')
    await flushPromises()
    expect(api.createNote).toHaveBeenLastCalledWith('s1', {
      anchorKind: 'table',
      anchorId: ANCHOR.id,
      body: 'a reply',
      parentId: 'theirs',
    })

    vi.mocked(api.updateNote).mockResolvedValue(note('theirs', 'bob'))
    await buttons(w, en['notes.item.resolve'])[1].trigger('click')
    await flushPromises()
    expect(api.updateNote).toHaveBeenCalledWith('theirs', { resolved: true })
  })

  it('lets a moderator edit and delete others’ notes', async () => {
    signIn('admin', ADMIN)
    const w = await thread([note('theirs', 'bob')])
    expect(buttons(w, en['notes.item.edit'])).toHaveLength(1)
    expect(buttons(w, en['notes.item.delete'])).toHaveLength(1)
  })

  it('gives a reader without note.write nothing to write with', async () => {
    signIn('reader', ['project.view'])
    const w = await thread([note('n1', 'bob', { replies: [note('r1', 'bob', { parentId: 'n1' })] })])
    expect(w.find('form').exists()).toBe(false)
    expect(buttons(w, en['notes.item.reply'])).toHaveLength(0)
    expect(buttons(w, en['notes.item.resolve'])).toHaveLength(0)
  })

  it('gives replies no Reply or Resolve', async () => {
    signIn('alice', VIEWER)
    const w = await thread([note('n1', 'bob', { replies: [note('r1', 'alice', { parentId: 'n1' })] })])
    const reply = w.find('.note--reply')
    expect(reply.text()).toContain(en['notes.item.edit'])
    expect(reply.text()).not.toContain(en['notes.item.reply'])
    expect(reply.text()).not.toContain(en['notes.item.resolve'])
  })

  it('asks before deleting, and cancel sends nothing', async () => {
    signIn('alice', VIEWER)
    const w = await thread([note('mine', 'alice')])
    await buttons(w, en['notes.item.delete'])[0].trigger('click')
    const dialog = w.find('[role="alertdialog"]')
    expect(dialog.text()).toContain(en['notes.delete.withReplies'])
    await dialog.findAll('button').find((b) => b.text() === en['confirm.cancel'])!.trigger('click')
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(api.deleteNote).not.toHaveBeenCalled()
  })

  it('submits on Ctrl+Enter and disables the button while pending', async () => {
    signIn('alice', VIEWER)
    const w = await thread([])
    let finish!: (n: Note) => void
    vi.mocked(api.createNote).mockReturnValue(new Promise((r) => (finish = r)))

    const box = w.find('form textarea')
    await box.setValue('quick')
    await box.trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(api.createNote).toHaveBeenCalledTimes(1)
    expect(buttons(w, en['notes.composer.add'])[0].attributes('disabled')).toBeDefined()

    finish(note('n1', 'alice'))
    await flushPromises()
    expect(buttons(w, en['notes.composer.add'])[0].attributes('disabled')).toBeDefined() // empty draft
    expect((box.element as HTMLTextAreaElement).value).toBe('')
  })

  it('closes on Escape', async () => {
    signIn('alice', VIEWER)
    const w = await thread([])
    await w.find('section').trigger('keydown', { key: 'Escape' })
    expect(w.emitted('close')).toHaveLength(1)
  })
})

describe('NotesButton', () => {
  it('is hidden at zero for a reader without note.write', async () => {
    signIn('reader', ['project.view'])
    const w = mount(NotesButton, { props: { anchor: ANCHOR } })
    mounted.push(w)
    await flushPromises()
    expect(w.find('button').exists()).toBe(false)
  })

  it('is shown at zero for a writer, and opens the thread', async () => {
    signIn('alice', VIEWER)
    vi.mocked(api.listNotes).mockResolvedValue({ notes: [] })
    const w = mount(NotesButton, { props: { anchor: ANCHOR }, attachTo: document.body })
    mounted.push(w)
    await flushPromises()
    await w.find('button').trigger('click')
    await flushPromises()
    expect(w.emitted('open')?.[0]).toEqual([ANCHOR])
    expect(w.findComponent(NotesThread).exists()).toBe(true)
    expect(api.listNotes).toHaveBeenCalledWith('s1', 'table', ANCHOR.id)
  })

  it('shows the open count, with resolved in the tooltip', async () => {
    signIn('reader', ['project.view'])
    vi.mocked(api.noteCounts).mockResolvedValue({
      counts: [{ anchorKind: 'table', anchorId: ANCHOR.id, open: 2, resolved: 1 }],
    })
    const w = mount(NotesButton, { props: { anchor: ANCHOR } })
    mounted.push(w)
    await flushPromises()
    expect(w.find('.count').text()).toBe('2')
    expect(w.find('button').attributes('title')).toBe('2 open notes · 1 resolved')
  })
})
