/**
 * Reads a documentation directory from the user's local disk.
 *
 * Two paths, because browsers disagree:
 *  - Chromium exposes the File System Access API, which gives a real OS
 *    directory picker and lets us walk the tree ourselves.
 *  - Everything else falls back to <input webkitdirectory>, which yields a flat
 *    FileList whose entries carry a webkitRelativePath.
 *
 * Both are normalised to the same shape: paths relative to the selected
 * directory, with the directory's own name stripped, so "model-docs/domain_one/
 * fact_primary.md" and "domain_one/fact_primary.md" produce identical table IDs no
 * matter which picker ran.
 */

import { ref } from 'vue'
import type { IngestFile } from '../api/types'
import { translate as t, translateCount as tn } from '../i18n'

/** Directories that never hold documentation and would only slow the walk. */
const SKIP_DIRS = new Set(['.git', 'node_modules', '.venv', '__pycache__', 'dist', 'build', '.next'])

/** Guard against a mis-click on a huge tree locking the tab up. */
const MAX_FILES = 5000
const MAX_TOTAL_BYTES = 48 * 1024 * 1024
const MAX_TOTAL_MB = MAX_TOTAL_BYTES / (1024 * 1024)

/**
 * Progress and error text is resolved as it is assigned rather than held as a
 * key and translated on read, unlike the workspace store's banners. Everything
 * here belongs to one action the reader just started and is gone within a few
 * seconds of it finishing; there is no window in which a language change could
 * find one of these strings still on screen.
 */

export interface PickedDirectory {
  name: string
  files: IngestFile[]
  /** Files seen but not read, e.g. non-markdown or skipped directories. */
  ignored: number
}

/** Feature-detects the native directory picker. */
export function supportsNativePicker(): boolean {
  return typeof (window as any).showDirectoryPicker === 'function'
}

export function useDirectoryPicker() {
  const reading = ref(false)
  const progress = ref(0)
  const progressLabel = ref('')
  const error = ref<string | null>(null)

  function reset() {
    reading.value = false
    progress.value = 0
    progressLabel.value = ''
    error.value = null
  }

  /** Opens the native OS directory picker and reads every .md file beneath it. */
  async function pickNative(): Promise<PickedDirectory | null> {
    error.value = null
    let handle: any
    try {
      handle = await (window as any).showDirectoryPicker({ mode: 'read' })
    } catch (e: any) {
      // AbortError means the user closed the picker; that is not a failure.
      if (e?.name === 'AbortError') return null
      error.value = e?.message ?? t('picker.error.open')
      return null
    }

    reading.value = true
    progress.value = 0
    progressLabel.value = t('picker.scanning')

    try {
      const files: IngestFile[] = []
      let ignored = 0
      let bytes = 0

      const walk = async (dir: any, prefix: string): Promise<void> => {
        for await (const [name, entry] of dir.entries()) {
          if (name.startsWith('.')) {
            ignored++
            continue
          }
          if (entry.kind === 'directory') {
            if (SKIP_DIRS.has(name)) {
              ignored++
              continue
            }
            await walk(entry, prefix ? `${prefix}/${name}` : name)
            continue
          }
          if (!name.toLowerCase().endsWith('.md')) {
            ignored++
            continue
          }
          if (files.length >= MAX_FILES) {
            throw new Error(t('picker.error.tooManyFiles', { max: MAX_FILES }))
          }

          const file: File = await entry.getFile()
          bytes += file.size
          if (bytes > MAX_TOTAL_BYTES) {
            throw new Error(t('picker.error.tooLarge', { mb: MAX_TOTAL_MB }))
          }

          const path = prefix ? `${prefix}/${name}` : name
          files.push({ path, content: await file.text() })
          progress.value = files.length
          progressLabel.value = tn('picker.read', files.length)
        }
      }

      await walk(handle, '')
      return { name: handle.name as string, files, ignored }
    } catch (e: any) {
      error.value = e?.message ?? t('picker.error.read')
      return null
    } finally {
      reading.value = false
    }
  }

  /**
   * Reads a FileList produced by <input webkitdirectory>. The browser prefixes
   * every path with the chosen directory's own name, which is stripped so the
   * result matches the native picker's output.
   */
  async function readFileList(list: FileList | null): Promise<PickedDirectory | null> {
    error.value = null
    if (!list || list.length === 0) return null

    reading.value = true
    progress.value = 0
    progressLabel.value = t('picker.reading')

    try {
      const all = Array.from(list)
      const rootName = deriveRootName(all)

      const markdown = all.filter((f) => {
        const rel = relativePath(f, rootName)
        if (!rel.toLowerCase().endsWith('.md')) return false
        const segments = rel.split('/')
        if (segments.some((s) => SKIP_DIRS.has(s) || s.startsWith('.'))) return false
        return true
      })

      if (markdown.length === 0) {
        error.value = t('picker.error.noMarkdown')
        return null
      }
      if (markdown.length > MAX_FILES) {
        error.value = t('picker.error.tooManyFiles', { max: MAX_FILES })
        return null
      }

      const totalBytes = markdown.reduce((n, f) => n + f.size, 0)
      if (totalBytes > MAX_TOTAL_BYTES) {
        error.value = t('picker.error.tooLarge', { mb: MAX_TOTAL_MB })
        return null
      }

      const files: IngestFile[] = []
      for (const f of markdown) {
        files.push({ path: relativePath(f, rootName), content: await f.text() })
        progress.value = files.length
        progressLabel.value = t('picker.readOf', { n: files.length, total: markdown.length })
      }

      return { name: rootName || 'documentation', files, ignored: all.length - markdown.length }
    } catch (e: any) {
      error.value = e?.message ?? t('picker.error.read')
      return null
    } finally {
      reading.value = false
    }
  }

  return { reading, progress, progressLabel, error, reset, pickNative, readFileList }
}

/** The common first path segment is the directory the user actually chose. */
function deriveRootName(files: File[]): string {
  const first = (files[0] as any).webkitRelativePath as string | undefined
  if (!first) return ''
  const head = first.split('/')[0]
  if (!head) return ''
  const shared = files.every((f) => {
    const p = (f as any).webkitRelativePath as string | undefined
    return !p || p.split('/')[0] === head
  })
  return shared ? head : ''
}

function relativePath(f: File, rootName: string): string {
  const raw = ((f as any).webkitRelativePath as string | undefined) || f.name
  if (rootName && raw.startsWith(`${rootName}/`)) {
    return raw.slice(rootName.length + 1)
  }
  return raw
}
