# dim_track

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_track` |
| **Type** | Dimension |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | One row per track. [JP] 楽曲ごとに 1 行。 |
| **Update Frequency** | Daily [JP] 毎日 |
| **Layer** | Star Schema (proposed) |

Every track the store can sell, flattened so that the album title sits beside the track rather than in a table of its own. The artist and the genre stay as keys, because both are read on their own elsewhere. [JP] ストアが販売しうるすべての楽曲。アルバム名は独立したテーブルに置かず、楽曲の隣にフラット化しています。アーティストとジャンルはそれぞれ単独で参照されるため、キーのまま残しています。

## Columns

| Column | Type | Description |
|---|---|---|
| `track_id` | STRING | Track identifier (PK) [JP] 楽曲識別子（主キー） |
| `track_name` | STRING | Track title as printed on the release [JP] リリース時に印字された楽曲名 |
| `album_title` | STRING | Album the track was released on [JP] 楽曲が収録されたアルバム名 |
| `artist_id` | STRING | Recording artist (FK) [JP] 録音アーティスト（外部キー） |
| `genre_id` | STRING | Genre the track is filed under (FK) [JP] 楽曲が分類されるジャンル（外部キー） |
| `media_type` | STRING | Format the track is sold in [JP] 販売フォーマット |
| `milliseconds` | INT64 | Running time [JP] 再生時間 |
| `unit_price` | FLOAT64 | List price of one track [JP] 楽曲 1 件の定価 |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `track_id` | `chinook.stg_tracks` | `track_id` | Primary Key [JP] 主キー |
| `track_name` | `chinook.stg_tracks` | `name` | |
| `album_title` | `chinook.stg_albums` | `title` | Joined on `album_id` [JP] `album_id` で結合 |
| `artist_id` | `chinook.stg_albums` | `artist_id` | Foreign Key [JP] 外部キー |
| `genre_id` | `chinook.stg_tracks` | `genre_id` | Foreign Key [JP] 外部キー |
| `media_type` | `chinook.stg_media_types` | `name` | |
| `milliseconds` | `chinook.stg_tracks` | `milliseconds` | |
| `unit_price` | `chinook.stg_tracks` | `unit_price` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_artist` | `artist_id = artist_id` | Many-to-one |
| `dim_genre` | `genre_id = genre_id` | Many-to-one |

## Notes / Caveats

- A track with no album — a single — carries a null `album_title` rather than a placeholder row. [JP] アルバムを持たないシングル曲は、ダミー行ではなく `album_title` を NULL にします。
- `unit_price` is the list price, not what any invoice line was actually charged. Sales keeps the charged price on its own fact. [JP] `unit_price` は定価であり、実際に請求された金額ではありません。請求額は販売コンテキストのファクトが保持します。
