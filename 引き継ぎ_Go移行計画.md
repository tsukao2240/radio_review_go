# 引き継ぎ資料 — Go移行計画（サブエージェント並列開発）

作成日: 2026-03-22

---

## 1. 現在のPHPプロジェクト状態

### リポジトリ
- PHP版: `~/project/radio_review/`
- Go版作業場所: `~/project/radio_review_go/`

### ブランチ状態（PHP版）

| ブランチ | 状態 |
|---------|------|
| `master` | CI通過済み・本番相当 |
| `feature/improvement2` | セキュリティ/品質改善8件・未マージ |

`feature/improvement2` の内容:
1. 録音情報にowner_keyを追加しユーザー間アクセス制御
2. 録音開始APIにバリデーション追加（422対応）
3. deleteRecordingの未定義変数$filename修正
4. 番組詳細画面の残存alert()をToast通知に統一
5. CSPからunsafe-eval・img-srcのhttp:削除
6. SSL verify:falseに意図説明コメント追加
7. 検索キャッシュTTL 1時間→5分
8. APP_ENVデフォルトをproductionからlocalに変更

---

## 2. Go版の技術スタック（確定済み）

| レイヤー | 採用技術 |
|---------|---------|
| Webフレームワーク | Chi or Echo |
| テンプレート | **html/template**（Go標準） |
| ORM | sqlx |
| Redis | go-redis |
| 認証 | gorilla/sessions + bcrypt |
| 設定 | godotenv |
| フロントエンド | Vite 6 + React 18（PHP版から流用） |

---

## 3. ディレクトリ構成（Go版）

```
radio_review_go/
├── cmd/server/main.go
├── internal/
│   ├── handler/        # RadioRecordingController等に相当
│   ├── service/        # RadikoApiService等に相当
│   ├── repository/     # DB操作
│   ├── middleware/     # 認証・CSP・レートリミット
│   └── model/         # 構造体定義
├── pkg/
│   └── radiko/
│       ├── client.go   # Radiko認証（最重要）
│       └── hls.go      # HLS並列ダウンロード
└── web/
    ├── templates/      # html/templateファイル
    └── static/         # Viteビルド成果物
```

---

## 4. サブエージェント並列開発の進め方

### 基本方針

**設計（直列）→ 実装（並列）→ 統合（直列）** の3フェーズで進める。

設計が固まる前に並列で走らせると噛み合わなくて後でやり直しになるため、
インターフェースと構造体定義を先に確定してから並列実装に入ること。

---

### フェーズ1: 骨格づくり（直列・1セッション）

Claudeへの指示例：
```
radio_review_go/ に以下の骨格を作って。
PHPの実装は ../radio_review/ を参照してよい。

1. go.mod（モジュール名: github.com/yourname/radio_review_go）
2. internal/model/ の全構造体（User, Post, FavoriteProgram, RecordingInfo等）
3. 各パッケージのinterfaceだけ定義したファイル
   - internal/service/radiko.go（interfaceのみ）
   - internal/repository/user.go（interfaceのみ）
   - pkg/radiko/client.go（interfaceのみ）
```

---

### フェーズ2: 並列実装（サブエージェント3〜4本同時）

骨格（インターフェース）が固まったら以下を同時に指示する：

**Claudeへの指示例（1メッセージで送る）：**
```
以下を並列で実装して。各Agentは独立したファイルを担当し、
interfaceに従って実装すること。PHPの実装は ../radio_review/ を参照してよい。

Agent1（Radiko認証）:
  pkg/radiko/client.go を実装。
  参照: ../radio_review/app/Http/Controllers/RadioRecordingController.php
  の getRadikoAuthToken() メソッド（210行目付近）

Agent2（HLS並列ダウンロード）:
  pkg/radiko/hls.go を実装。
  errgroup + semaphore で最大10並列。
  参照: ../radio_review/app/Http/Controllers/RadioRecordingController.php
  の downloadHlsSegments() 付近

Agent3（DBモデル・マイグレーション）:
  internal/repository/ の実装とSQLマイグレーションファイル。
  参照: ../radio_review/database/migrations/

Agent4（ミドルウェア）:
  internal/middleware/ の認証・セキュリティヘッダー・レートリミット実装。
  参照: ../radio_review/app/Http/Middleware/
```

---

### フェーズ3: ハンドラー実装（並列）

```
以下を並列で実装して。

Agent1: internal/handler/recording.go
  参照: ../radio_review/app/Http/Controllers/RadioRecordingController.php

Agent2: internal/handler/broadcast.go + internal/service/radiko.go
  参照: ../radio_review/app/Http/Controllers/RadioBroadcastController.php
        ../radio_review/app/Services/RadikoApiService.php

Agent3: internal/handler/post.go + internal/handler/favorite.go
  参照: ../radio_review/app/Http/Controllers/PostController.php
        ../radio_review/app/Http/Controllers/FavoriteProgramController.php

Agent4: web/templates/ の全Bladeテンプレートをhtml/templateに変換
  参照: ../radio_review/resources/views/
```

---

### フェーズ4: 統合・テスト（直列）

```
cmd/server/main.go でルーティングを組み上げて、
go test ./... が通るようにして。
```

---

## 5. 重要な実装メモ

### Radiko認証（最重要・最複雑）

PHPの `getRadikoAuthToken()` の流れ（`RadioRecordingController.php` 210行目付近）:
1. `https://radiko.jp/v2/api/auth1` にGET
2. レスポンスヘッダーから `X-Radiko-AuthToken`, `X-Radiko-KeyLength`, `X-Radiko-KeyOffset` 取得
3. `storage/app/keys/radiko_auth_key.txt`（Base64）から部分キーを切り出し
4. `https://radiko.jp/v2/api/auth2` にGET（トークン + 部分キー + GPS座標）
5. RedisにTTL 55分でキャッシュ（キー: `radiko_auth_token_{areaId}`）

**注意**: `InsecureSkipVerify: true` はRadiko互換性のため意図的。

### HLS並列ダウンロード

```go
// Go版のイメージ
func downloadSegments(ctx context.Context, urls []string) ([][]byte, error) {
    results := make([][]byte, len(urls))
    g, ctx := errgroup.WithContext(ctx)
    sem := semaphore.NewWeighted(10)

    for i, url := range urls {
        i, url := i, url
        g.Go(func() error {
            sem.Acquire(ctx, 1)
            defer sem.Release(1)
            data, err := fetchSegment(url)
            if err != nil { return err }
            results[i] = data
            return nil
        })
    }
    return results, g.Wait()
}
```

### 録音のアクセス制御（owner_key）

Redisに保存する録音情報に `owner_key` を含める:
- ログイン済み: `user_{userID}`
- ゲスト: `session_{sessionID}`

一覧・履歴は自分のowner_keyと一致するものだけ返す。

---

## 6. DBスキーマ

### users
| カラム | 型 |
|-------|-----|
| id | bigint PK |
| name | varchar |
| email | varchar UNIQUE |
| email_verified_at | timestamp NULL |
| password | varchar (bcrypt) |
| remember_token | varchar NULL |
| created_at / updated_at | timestamp |

### posts（感想）
| カラム | 型 |
|-------|-----|
| id | bigint PK |
| user_id | bigint FK |
| program_id | bigint FK → radio_programs |
| program_title | varchar |
| title | varchar |
| body | text |
| rating | decimal(2,1) NULL（1.0〜5.0） |
| station_id | varchar NULL |
| likes_count / comments_count | int |
| created_at / updated_at | timestamp |

### radio_programs（Radiko APIキャッシュ）
| カラム | 型 |
|-------|-----|
| id | bigint PK |
| station_id | varchar |
| title | varchar |
| cast | text NULL |
| start / end | varchar（YYYYMMDDHHMM形式） |
| info / url / image | text NULL |

### favorite_programs
| カラム | 型 |
|-------|-----|
| id | bigint PK |
| user_id | bigint FK |
| station_id | varchar(50) |
| program_title | varchar(255) |
| broadcast_day | tinyint NULL（0=月〜6=日） |
| UNIQUE | (user_id, station_id, program_title, broadcast_day) |

### recording_schedules
| カラム | 型 |
|-------|-----|
| id | bigint PK |
| user_id | bigint FK |
| station_id | varchar |
| program_title | varchar |
| scheduled_start_time / end_time | datetime |
| status | enum(pending/recording/completed/failed/cancelled) |
| recording_id | varchar NULL |
| error_message | text NULL |

### その他
- `post_tags`: id, name(50), display_order
- `post_post_tag`（中間テーブル）: post_id, tag_id
- `post_likes`: id, post_id, user_id（UNIQUE）
- `post_comments`: id, post_id, user_id, body(text)
- `notifications`: user_id, type, data(JSON), read_at NULL

---

## 7. Redisキャッシュキー一覧

| キー | TTL | 内容 |
|-----|-----|------|
| `radiko_auth_token_{areaId}` | 55分 | Radiko認証トークン |
| `recording_{recordingId}` | 2時間 | 録音情報JSON（owner_key含む） |
| `recording_disk_usage` | 60秒 | ディスク使用状況 |
| `search_programs_{md5(keyword)}` | 5分 | 番組検索結果 |
| `weekly_schedule_{stationId}` | 30分 | 週間番組表 |

---

## 8. 環境変数（.env）

```env
APP_KEY=...
APP_ENV=local
APP_PORT=8080
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=radio_review
DB_USERNAME=...
DB_PASSWORD=...
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
RECORDING_STORAGE_PATH=storage/recordings
RECORDING_MAX_PARALLEL=10
RECORDING_CHUNK_DELAY=0
```
