# dim_artist

## Overview

| Property | Value |
|---|---|
| **Table Name** | `dim_artist` |
| **Type** | Dimension |
| **Domain** | Catalog |
| **Bounded Context** | Catalog |
| **Grain** | One row per recording artist. [JP] 録音アーティストごとに 1 行。 |
| **Update Frequency** | Daily [JP] 毎日 |
| **Layer** | Star Schema (proposed) |

The artist credited on the release. Chinook has no artist attributes beyond the name, and inventing some would be inventing data. [JP] リリースにクレジットされたアーティスト。Chinook には名前以外の属性がなく、無いものを作るのはデータの捏造にあたるため、そのままにしています。

## Columns

| Column | Type | Description |
|---|---|---|
| `artist_id` | STRING | Artist identifier (PK) [JP] アーティスト識別子（主キー） |
| `artist_name` | STRING | Name as credited [JP] クレジット表記の名前 |

## Column-Level Lineage

| Column | Source Table | Source Column | Notes |
|---|---|---|---|
| `artist_id` | `chinook.stg_artists` | `artist_id` | Primary Key [JP] 主キー |
| `artist_name` | `chinook.stg_artists` | `name` | |

## Relationships

| Related Table | Join Key | Relationship |
|---|---|---|
| `dim_track` | `artist_id = artist_id` | One-to-many |

## Notes / Caveats

- Compilations are credited to the album's own artist row, so a compilation's tracks all point at one artist that is not the performer. [JP] コンピレーションはアルバム側のアーティスト行にクレジットされるため、収録曲はすべて実演者ではない 1 つのアーティストを指します。
