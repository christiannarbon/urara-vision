# dim_genre

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_genre` |
| **Type** | Dimension |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | ジャンルごとに 1 行。 |
| **Update Frequency** | Daily [JP] 毎日 |
| **Layer** | Star Schema (proposed) |

ジャンルは楽曲の分類であり、店舗の棚に相当します。二十数行しかなく、増えることもほとんどありません。

## Columns

| Column | Type | Description |
|---|---|---|
| `genre_id` | STRING | Genre identifier (PK) [JP] ジャンル識別子（主キー） |
| `genre_name` | STRING | Genre as shown in the store [JP] ストア表示上のジャンル名 |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `genre_id` | `chinook.stg_genres` | `genre_id` | Primary Key [JP] 主キー |
| `genre_name` | `chinook.stg_genres` | `name` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_track` | `genre_id = genre_id` | One-to-many |

## Notes / Caveats

- This document's description and grain are written only in Japanese, with no tag and no English beside them. That is deliberate: an untagged field is primary-language text whatever script it is in, so every reader sees it in both languages rather than seeing nothing. [JP] このドキュメントの説明と粒度は、タグも英語も伴わない日本語だけで書かれています。これは意図的なものです。タグのないフィールドはどの文字体系であっても主要言語のテキストとして扱われ、どの読者にも空白ではなくそのまま表示されます。
