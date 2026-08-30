/**
 * Japanese.
 *
 * Typed as `Messages`, so leaving a key untranslated is a compile error rather
 * than a string that silently reverts to English in front of a reader.
 *
 * Two conventions the rest of this file follows:
 *
 *   Domain vocabulary stays in the reader's language, not in transliteration.
 *   A dimension table is a ディメンションテーブル because that is what the
 *   documents these models come from call one; writing ディメンション for the
 *   role and テーブル for the noun everywhere else would read as two
 *   vocabularies.
 *
 *   Counted strings still carry a `.one` and an `.other`, and they are the
 *   same sentence. Japanese does not mark plural, so `plural` below answers
 *   `other` for every number and the `.one` entries are never read -- they
 *   exist because the key set is shared, and the duplication is cheaper than a
 *   catalogue type that has to describe which locales count.
 */
import type { Messages } from './en'

export const messages: Messages = {
  'locale.en': 'English',
  'locale.ja': '日本語',

  'app.title': 'Urara Vision — データモデルエクスプローラ',

  // Topbar
  'topbar.home': '取り込み一覧に戻る',
  'topbar.search': '検索',
  'topbar.search.title': '検索（⌘K）',
  'topbar.diagnostics': '診断',
  'topbar.diagnostics.title': '診断',
  'topbar.diagnostics.titleAttention': '診断 — 確認したほうがよい項目があります',
  'topbar.diagnostics.flag': '診断に確認が必要です',
  'topbar.newIngest': '新しい取り込み',

  // The banners under the topbar
  'banner.dismiss': '閉じる',
  'banner.review': '確認する',
  'banner.parseFailures.one': '{n} 件のドキュメントを解析できず、スキップしました。',
  'banner.parseFailures.other': '{n} 件のドキュメントを解析できず、スキップしました。',
  'banner.parseFailuresDetail.one': 'そこに記載されている内容は、このモデルには含まれていません。',
  'banner.parseFailuresDetail.other': 'そこに記載されている内容は、このモデルには含まれていません。',

  // Theme picker
  'theme.label': 'テーマ',
  'theme.current': 'テーマ：{name}',
  'theme.paintings': '絵画',

  // API token gate
  'gate.title': 'このインスタンスにはトークンが必要です',
  'gate.intro.before':
    'API は共有トークンで保護されています。このデプロイの運用担当者に問い合わせてください。バックエンドの起動時に指定した',
  'gate.intro.after': 'です。',
  'gate.field': 'API トークン',
  'gate.placeholder': 'トークンを貼り付けてください',
  'gate.rejected':
    'このトークンは受け付けられませんでした。文字の欠落や余分な文字がないか確認してください。',
  'gate.checking': '確認中…',
  'gate.continue': '続ける',
  'gate.storage':
    'トークンはこのブラウザのローカルストレージに保存されるため、毎回入力する必要はありません。',
}

/** Japanese has no plural forms: one table and forty tables read alike. */
export function plural(_n: number): 'one' | 'other' {
  return 'other'
}
