# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 動作指針 (Filesystem First)

回答を生成する前に、必ず以下のステップを遵守してください。これにより、不要な推測を排除し、トークン消費を最小限に抑えます。

1. **ファイル構造の確認**: プロジェクトの構造が不明な場合は、まず `ls -R` や `find` を使用してディレクトリ構成を確認してください。
2. **情報の直接取得**: コードの内容や関数定義について推測で答えず、`grep` や `read_file` を使用して、必ず実際のソースコードを確認してください。
3. **最小限のコンテキスト抽出**: ファイル全体を読み込むのではなく、必要な範囲のみを特定して読み込むように努めてください。
4. **推測の排除**: 実装の詳細が不明な段階でコードを提案しないでください。まずfilesystemから事実を確認し、確証を得てから1回で正確な修正案を提示してください。

## コミット前のルール

**コードを修正した場合、コミットする前に以下を必ず順番に実行すること。**

### 手順

1. **未コミット変更の全確認**

```bash
git status
```

修正したファイルがすべてステージされているか確認すること。`git status` で表示された変更ファイルを漏れなくコミットに含めること。

2. **全テスト実行**

```bash
go test ./...
```

テストが全件パスしてからコミットすること。1件でも失敗した場合はコミット禁止。

3. **コミット**

`git status` の確認とテストパスの両方が完了してからコミットすること。

**`git push` は絶対に行わないこと。プッシュはユーザーが手動で行う。**

## テスト

```bash
# 全テスト実行
go test ./...

# 特定パッケージのみ
go test ./internal/handler/...
go test ./pkg/radiko/...

# 特定テスト
go test -run TestRadikoAuth ./pkg/radiko/

# カバレッジ確認
go test -cover ./...
```

## 開発コマンド

```bash
# 開発サーバー起動
go run cmd/server/main.go

# ビルド
go build -o bin/server cmd/server/main.go

# 依存関係の整理
go mod tidy

# コードフォーマット（コミット前に実行）
gofmt -w .

# 静的解析
go vet ./...
```

## プロジェクト概要

ラジオ番組の番組表表示・感想投稿・タイムフリー録音機能を持つWebアプリケーション。
PHP/Laravel版（`../radio_review/`）をGoで書き直したもの。Radiko APIと連携し、番組スケジュール取得や過去放送の録音が可能。

詳細な移行計画・DBスキーマ・Redisキャッシュ構造: `引き継ぎ_Go移行計画.md`

## 技術スタック

- **言語:** Go 1.22+
- **Webフレームワーク:** Chi or Echo
- **テンプレート:** html/template（Go標準ライブラリ）
- **DB:** MySQL 8.0 + sqlx
- **キャッシュ:** Redis + go-redis
- **認証:** gorilla/sessions + bcrypt
- **設定:** godotenv（.envファイル）
- **フロントエンド:** Vite 6 + React 18（radio_review から流用）
- **音声処理:** FFmpeg（os/exec で呼び出し）

## ディレクトリ構成

```
radio_review_go/
├── cmd/server/main.go          # エントリポイント・ルーティング
├── internal/
│   ├── handler/                # HTTPハンドラー（Controllerに相当）
│   ├── service/                # ビジネスロジック
│   ├── repository/             # DB操作
│   ├── middleware/             # 認証・CSP・レートリミットなど
│   └── model/                 # 構造体定義
├── pkg/
│   └── radiko/
│       ├── client.go           # Radiko認証（最重要）
│       └── hls.go              # HLSセグメント並列ダウンロード
└── web/
    ├── templates/              # html/templateファイル
    └── static/                 # Viteビルド成果物・SW・manifest
```

## アーキテクチャ上の注意点

### Radiko認証
- `pkg/radiko/client.go` で実装
- SSL検証は `InsecureSkipVerify: true`（Radiko API互換性のため意図的）
- 認証トークンはRedisに55分キャッシュ（キー: `radiko_auth_token_{areaId}`）
- 認証キーファイル: `storage/keys/radiko_auth_key.txt`（Base64）

### HLS並列ダウンロード
- `pkg/radiko/hls.go` で実装
- `errgroup` + `semaphore` で最大10並列（環境変数 `RECORDING_MAX_PARALLEL` で変更可能）

### 録音ファイルのアクセス制御
- 録音情報をRedisに保存する際、`owner_key`（`session_{id}` or `user_{id}`）を含める
- 一覧・履歴表示は自分の `owner_key` と一致するものだけ返す

### html/template の注意
- `{{ }}` でエスケープ、`template.HTML` で意図的な生HTML出力
- Bladeの `@if` → `{{ if }}...{{ end }}`
- Bladeの `@auth` → セッションからユーザー取得して条件分岐

## 環境変数（.env）

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
```
