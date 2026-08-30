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
  'language.label': '言語',
  'language.current': '言語：{name}',

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

  // The entry screen
  'welcome.title': 'データモデルビジュアライザ',
  'welcome.intro':
    'テーブルドキュメントのディレクトリを指定してください。Markdown を解析してテーブル間のリレーションを解決し、モデルを探索できるグラフとして描画します。',
  'welcome.working': '処理中…',
  'welcome.dropzone.title': 'ドキュメントのディレクトリを選択してください',
  'welcome.dropzone.hint.before': '配下のすべての',
  'welcome.dropzone.hint.after':
    'ファイルをブラウザ内で読み取り、パーサーに送信します。ディスクへの書き込みは行いません。',
  'welcome.choose': 'フォルダを選択…',
  'welcome.useFileInput': 'ファイル入力を使う',
  'welcome.noPicker':
    'お使いのブラウザはディレクトリピッカーに対応していないため、ファイル入力を使用します。動作は同じです。',
  'welcome.recent': 'これまでの取り込み',
  'welcome.stats.tables.one': 'テーブル {n} 件',
  'welcome.stats.tables.other': 'テーブル {n} 件',
  'welcome.stats.domains.one': 'ドメイン {n} 件',
  'welcome.stats.domains.other': 'ドメイン {n} 件',
  'welcome.delete.title': 'このスナップショットを削除',
  'welcome.delete.label': 'スナップショットを削除',

  // Reading a directory
  'picker.scanning': 'ディレクトリを走査しています…',
  'picker.reading': 'ドキュメントを読み込んでいます…',
  'picker.read.one': '{n} 件のドキュメントを読み込みました…',
  'picker.read.other': '{n} 件のドキュメントを読み込みました…',
  'picker.readOf': '{total} 件中 {n} 件のドキュメントを読み込みました…',
  'picker.error.open': 'ディレクトリピッカーを開けませんでした。',
  'picker.error.read': '選択したディレクトリを読み取れませんでした。',
  'picker.error.noMarkdown': 'このディレクトリには Markdown ファイルがありません。',
  'picker.error.tooManyFiles':
    'このディレクトリには {max} 件を超える Markdown ファイルがあります。より狭い範囲を選択してください。',
  'picker.error.tooLarge': '選択したドキュメントはアップロード上限の {mb} MB を超えています。',

  // What the workspace is doing, and what went wrong
  'status.parsing.one': '{n} 件のドキュメントを解析しています…',
  'status.parsing.other': '{n} 件のドキュメントを解析しています…',
  'status.loading': 'モデルを読み込んでいます…',
  'error.unknown': '問題が発生しました。',
  'error.unreachable':
    'バックエンドに接続できません。API が起動していて到達可能か確認してください。',
  'error.tokenRejected': 'この API にはトークンが必要ですが、指定されたトークンは受け付けられませんでした。',
  'error.requestFailed': 'リクエストが失敗しました（ステータス {status}）。',

  // The role vocabulary
  'role.fact': 'ファクト',
  'role.factless': 'ファクトレスファクト',
  'role.dimension': 'ディメンション',
  'role.outrigger': 'アウトリガー',
  'role.bridge': 'ブリッジ',
  'role.junk': 'ジャンクディメンション',
  'role.degenerate': '縮退ディメンション',
  'role.hub': 'ハブ',
  'role.link': 'リンク',
  'role.satellite': 'サテライト',
  'role.pit': 'ポイントインタイム',
  'role.entity': 'エンティティ',
  'role.associative': '関連エンティティ',
  'role.lookup': 'ルックアップ',
  'role.reference': 'リファレンス',
  'role.unknown': '不明',

  // The three layouts
  'layout.force': 'フォース',
  'layout.force.hint': '読む方向を持たない自由な配置です。テーブルをドメインごとにまとめます。',
  'layout.layered': 'レイヤー',
  'layout.layered.hint':
    'ジョインに沿ってテーブルを段に並べます — 正規化の深さ、あるいは Data Vault の層。ドメインごとにまとめることはできません。',
  'layout.radial': '放射',
  'layout.radial.hint':
    'ジョインの多いテーブルを中心に置き、外側へ広げます。ドメインごとにまとめることはできません。',

  // Left rail
  'rail.label': 'フィルタとテーブル',
  'rail.stats.tables': 'テーブル',
  'rail.stats.domains': 'ドメイン',
  'rail.stats.columns': 'カラム',
  'rail.stats.sources': 'ソース',
  'rail.view': '表示',
  'rail.view.label': '表示モード',
  'rail.view.whole': 'モデル全体',
  'rail.view.focused': '絞り込み',
  'rail.view.focused.title': '選択したテーブルとその隣接テーブルだけを表示します',
  'rail.view.focused.disabled': '先にテーブルを選択してください',
  'rail.layout.label': 'レイアウト',
  'rail.depth': '深さ',
  'rail.showSources': '上流のソースモデルを表示',
  'rail.crossDomainOnly': 'ドメインをまたぐジョインのみ',
  'rail.filters': 'フィルタ',
  'rail.clear': 'クリア',
  'rail.role.fromDocuments': 'ドキュメントから読み取ったロール：{role}',
  'rail.tables': 'テーブル（{n}）',
  'rail.tables.filter': 'テーブルを絞り込み…',
  'rail.tables.filter.label': 'テーブルを絞り込み',
  'rail.conformed': '準拠：他のドメインでも定義されています',
  'rail.noMatches': '一致するテーブルはありません。',

  // The canvas
  'canvas.label': 'テーブル関連グラフ',
  'canvas.building': 'グラフを構築しています…',
  'canvas.layingOut': '配置しています…',
  'canvas.empty.title': '描画するものがありません',
  'canvas.empty.body': '現在のフィルタに一致するテーブルはありません。',
  'canvas.controls': 'グラフ操作',
  'canvas.zoomIn': '拡大',
  'canvas.zoomOut': '縮小',
  'canvas.fit': '全体を表示',
  'canvas.relayout': 'レイアウトを再実行',
  'canvas.group': 'ドメインごとにまとめる',
  'canvas.ungroup': 'ドメインのまとまりを解除',
  'canvas.group.unavailable': 'まとめられるのは {layout} レイアウトのときだけです',
  'canvas.legend': '凡例',
  'canvas.legend.source': 'ソースモデル',
  'canvas.legend.crossDomain': 'ドメインをまたぐジョイン',
  'canvas.legend.cluster': 'ドメインのまとまり',

  // Detail pane
  'detail.label': 'テーブルの詳細',
  'detail.loading': 'テーブルを読み込んでいます…',
  'detail.sourceOnly':
    'これはカラムリネージから参照されている上流のソースモデルです。このスナップショットには専用のテーブルドキュメントがありません。',
  'detail.empty': 'グラフ上のテーブルを選択すると、説明・カラム・リネージが表示されます。',
  'detail.empty.hint': 'ノードをダブルクリックすると、そのノードを中心にグラフを表示します。',
  'detail.conformed': '準拠',
  'detail.close': '詳細を閉じる',
  'detail.focusHere': 'ここを中心に表示',
  'detail.tab.overview': '概要',
  'detail.tab.columns': 'カラム',
  'detail.tab.joins': 'ジョイン',
  'detail.tab.lineage': 'リネージ',
  'detail.grain': '粒度',
  'detail.type': '種別',
  'detail.domain': 'ドメイン',
  'detail.updated': '更新頻度',
  'detail.layer': 'レイヤー',
  'detail.source': 'ソース',
  'detail.keys': 'キー',
  'detail.alsoDefinedIn': '他に定義されているドメイン',
  'detail.alsoDefinedIn.note':
    'このテーブル名は他のドメインにも存在します。定義が異なる場合があるため、診断パネルで差異を確認してください。',
  'detail.notes': '注記と注意点',
  'detail.columns.filter': 'カラムを絞り込み…',
  'detail.columns.filter.label': 'カラムを絞り込み',
  'detail.columns.noMatches': 'この条件に一致するカラムはありません。',
  'detail.columns.from': '参照元',
  'detail.columns.derived': '導出',
  'detail.joins.declared': 'このテーブルが宣言しているジョイン',
  'detail.joins.none': 'このテーブルはリレーションを宣言していません。',
  'detail.joins.crossDomain': 'ドメイン横断',
  'detail.joins.boundTo': '{target} に解決されました。{alternatives} にも定義されています',
  'detail.joins.unresolved': '未解決の参照',
  'detail.joins.noDocument': 'ドキュメントなし',
  'detail.joins.prose': '文章',
  'detail.joins.referencedBy': 'このテーブルを参照しているテーブル',
  'detail.lineage.upstream': '上流のソースモデル',
  'detail.lineage.none': 'このテーブルにはカラム単位のリネージが記載されていません。',
  'detail.lineage.columns.one': 'カラム {n} 件',
  'detail.lineage.columns.other': 'カラム {n} 件',
  'detail.lineage.more': '他 {n} 件',
  'detail.siblings': 'ソースを共有しているテーブル',
  'detail.siblings.note':
    'これらのテーブルは少なくとも 1 つの上流モデルを共有しているため、上流の変更はこれらにも影響する可能性があります。',
  'detail.siblings.shared.one': '共有ソース {n} 件',
  'detail.siblings.shared.other': '共有ソース {n} 件',

  // Search overlay
  'search.label': 'テーブルを検索',
  'search.placeholder': 'テーブル・カラム・説明を検索…',
  'search.input.label': '検索',
  'search.hint': '入力すると、テーブル名・粒度・説明・カラム名を横断して検索します。',
  'search.searching': '検索しています…',
  'search.noMatches': '一致するものはありません。',
  'search.key.navigate': '移動',
  'search.key.open': '開く',
  'search.key.close': '閉じる',

  // Diagnostics panel
  'diagnostics.label': 'ドキュメントの診断',
  'diagnostics.title': '診断',
  'diagnostics.subtitle': '解析および解決の過程で見つかった問題です。',
  'diagnostics.close': '診断を閉じる',
  'diagnostics.severity.all': 'すべて',
  'diagnostics.severity.error': 'エラー',
  'diagnostics.severity.warning': '警告',
  'diagnostics.severity.info': '情報',
  'diagnostics.empty':
    '報告する問題はありません。すべてのドキュメントが解析され、すべての参照が解決されました。',
  'diagnostics.unparsed': '解析できなかったドキュメント',
  'diagnostics.unparsed.note':
    'これらのファイルはすべてスキップされたため、その内容はモデルに反映されていません。',
  'diagnostics.findings': 'モデルに関する指摘',
  'diagnostics.findings.note':
    '以下はすべて正常に解析されています。リゾルバが確認したほうがよい点を検出しました。',

  // What each diagnostic code means
  'diagnostic.unresolved_reference.title': '未解決の参照',
  'diagnostic.unresolved_reference.blurb':
    'リレーションがドキュメントの存在しないテーブルを指しています。ドキュメントが未作成か、名前が誤っています。',
  'diagnostic.cross_domain_reference.title': 'ドメインをまたぐ参照',
  'diagnostic.cross_domain_reference.blurb':
    '自ドメインにドキュメントのないディメンションへジョインしているため、他ドメインの準拠ディメンションに解決されました。',
  'diagnostic.conformed_drift.title': '準拠ディメンションの不一致',
  'diagnostic.conformed_drift.blurb':
    '同じテーブル名がドメインごとに異なる内容で記述されています。準拠ディメンションは一致しているべきです。',
  'diagnostic.unmatched_join_key.title': '一致しないジョインキー',
  'diagnostic.unmatched_join_key.blurb':
    'ジョインキーが、どちらのテーブルにも記載のないカラムを指しています。',
  'diagnostic.undocumented_lineage.title': '記載のないリネージ',
  'diagnostic.undocumented_lineage.blurb':
    'Source Table 欄がモデル名ではなく文章（「not available」「N/A」など）になっているカラムです。これらはリネージグラフから除外されます。',
  'diagnostic.narrative_reference.title': '文章による参照',
  'diagnostic.narrative_reference.blurb':
    'リレーション欄がテーブル名ではなく文章になっているため、描画できませんでした。',
  'diagnostic.isolated_fact.title': '孤立したファクト',
  'diagnostic.isolated_fact.blurb':
    'ファクトテーブルが、解決可能なディメンションへのリレーションを 1 つも宣言していません。',
  'diagnostic.missing_domain_index.title': 'ドメインインデックスの欠落',
  'diagnostic.missing_domain_index.blurb':
    'テーブルドキュメントはあるものの、インデックスドキュメントがないディレクトリがあります。',
  'diagnostic.empty_domain.title': '空のドメイン',
  'diagnostic.empty_domain.blurb':
    'ドメインインデックスは存在しますが、そのディレクトリにテーブルドキュメントがありません。',
  'diagnostic.no_columns.title': 'カラムのないテーブル',
  'diagnostic.no_columns.blurb': 'テーブルドキュメントにカラムが 1 つも記載されていません。',
  'diagnostic.name_filename_mismatch.title': 'テーブル名とファイル名の不一致',
  'diagnostic.name_filename_mismatch.blurb':
    '宣言されたテーブル名がファイル名と異なります。',
  'diagnostic.unrecognised_document.title': '認識できないドキュメント',
  'diagnostic.unrecognised_document.blurb':
    'テーブルにもドメインインデックスにも当てはまらない Markdown ファイルです。すべてスキップされ、その内容はモデルに反映されていません。',
  'diagnostic.empty_document.title': '空のドキュメント',
  'diagnostic.empty_document.blurb':
    '内容が空の Markdown ファイルです。すべてスキップされました。',

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
