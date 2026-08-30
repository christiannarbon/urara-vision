/**
 * Reading a documentation directory, through both pickers.
 *
 * The two of them share nothing but their output shape, so the manifest rule
 * is asserted against each in turn: a directory without a projectmeta.toml at
 * its root is refused here, before an upload the backend would reject anyway,
 * and a manifest sitting one level down does not count as one.
 */

import { afterEach, describe, expect, it } from 'vitest'

import { useDirectoryPicker } from '../../src/composables/useDirectoryPicker'
import { messages as en } from '../../src/i18n/messages/en'

const MANIFEST = `[project]
name = "p"
version = "0.1.0"

[internationalization]
primary = "EN"
supported = ["EN"]
type = "inline"
`

/**
 * A File as <input webkitdirectory> hands it over: named by its full path.
 *
 * jsdom's File has no `text()`, which every browser that reaches this code
 * does, so the stand-in supplies one.
 */
function pickedFile(relativePath: string, content = '# doc\n'): File {
  const f = new File([content], relativePath.split('/').pop() ?? relativePath)
  Object.defineProperty(f, 'webkitRelativePath', { value: relativePath })
  Object.defineProperty(f, 'text', { value: async () => content })
  return f
}

function fileList(...files: File[]): FileList {
  return files as unknown as FileList
}

// --- the File System Access API, in as much shape as the walk uses ---------

function fsFile(content: string) {
  return {
    kind: 'file',
    async getFile() {
      return { size: content.length, text: async () => content }
    },
  }
}

function fsDir(name: string, entries: Record<string, unknown>) {
  return {
    kind: 'directory',
    name,
    async *entries() {
      for (const entry of Object.entries(entries)) yield entry
    },
  }
}

/** Installs a native picker that hands back one directory. */
function withNativePicker(handle: unknown) {
  ;(window as any).showDirectoryPicker = async () => handle
}

// Assignment rather than a mock, so it has to be taken back by hand: a spec
// that leaves one installed changes what the next one feature-detects.
afterEach(() => {
  delete (window as any).showDirectoryPicker
})

describe('the file input', () => {
  it('reads the manifest and keeps it out of the documents', async () => {
    const picker = useDirectoryPicker()
    const picked = await picker.readFileList(
      fileList(
        pickedFile('docs/projectmeta.toml', MANIFEST),
        pickedFile('docs/domain_one.md'),
        pickedFile('docs/domain_one/fact_primary.md'),
      ),
    )

    expect(picked).not.toBeNull()
    expect(picked!.manifest).toEqual({ path: 'projectmeta.toml', content: MANIFEST })
    expect(picked!.files.map((f) => f.path)).toEqual([
      'domain_one.md',
      'domain_one/fact_primary.md',
    ])
    expect(picker.error.value).toBeNull()
  })

  it('refuses a directory with no manifest, and says which file is missing', async () => {
    const picker = useDirectoryPicker()
    const picked = await picker.readFileList(fileList(pickedFile('docs/domain_one.md')))

    expect(picked).toBeNull()
    expect(picker.error.value).toBe(en['picker.error.noManifest'])
    expect(picker.reading.value).toBe(false)
  })

  it('does not accept a manifest from below the root', async () => {
    const picker = useDirectoryPicker()
    const picked = await picker.readFileList(
      fileList(
        pickedFile('docs/domain_one/projectmeta.toml', MANIFEST),
        pickedFile('docs/domain_one/fact_primary.md'),
      ),
    )

    expect(picked).toBeNull()
    expect(picker.error.value).toBe(en['picker.error.noManifest'])
  })
})

describe('the native picker', () => {
  it('reads the manifest from the root of the chosen directory', async () => {
    withNativePicker(
      fsDir('docs', {
        'projectmeta.toml': fsFile(MANIFEST),
        'domain_one.md': fsFile('# Domain One\n'),
        domain_one: fsDir('domain_one', { 'fact_primary.md': fsFile('# fact_primary\n') }),
      }),
    )

    const picker = useDirectoryPicker()
    const picked = await picker.pickNative()

    expect(picked).not.toBeNull()
    expect(picked!.name).toBe('docs')
    expect(picked!.manifest.content).toBe(MANIFEST)
    expect(picked!.files.map((f) => f.path)).toEqual([
      'domain_one.md',
      'domain_one/fact_primary.md',
    ])
  })

  it('refuses a directory with no manifest', async () => {
    withNativePicker(fsDir('docs', { 'domain_one.md': fsFile('# Domain One\n') }))

    const picker = useDirectoryPicker()
    const picked = await picker.pickNative()

    expect(picked).toBeNull()
    expect(picker.error.value).toBe(en['picker.error.noManifest'])
    expect(picker.reading.value).toBe(false)
  })

  it('does not accept a manifest from below the root', async () => {
    withNativePicker(
      fsDir('docs', {
        'domain_one.md': fsFile('# Domain One\n'),
        domain_one: fsDir('domain_one', { 'projectmeta.toml': fsFile(MANIFEST) }),
      }),
    )

    const picker = useDirectoryPicker()
    expect(await picker.pickNative()).toBeNull()
    expect(picker.error.value).toBe(en['picker.error.noManifest'])
  })
})
