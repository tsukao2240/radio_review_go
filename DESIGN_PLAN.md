# ウォームラジオデザイン 実装計画書

> 目的: 「AIが作りました感」の強いTailwind汎用デザインを、温かみのあるラジオ・アナログ感のあるデザインに刷新する

---

## 現状の技術スタック

- **CSS配信**: `web/static/app.css`（Viteビルド済みバンドル）
  - 中身: Toastify + Bootstrap 5.3.8 + Tailwindユーティリティ（コンパイル済み）
  - ビルドソースは見当たらない（pre-built扱い）
- **テンプレート**: `web/templates/**/*.html`（Go `html/template`）
  - Tailwindのユーティリティクラスを直接記述
  - `card-base` などカスタムユーティリティもあり
- **JS**: `web/static/app.js`（Viteビルド済み、Toast通知など）
- **ビルドシステム**: このディレクトリにはvite.config不在 → staticファイルを直接編集する方針

---

## 設計方針

### やること
- `web/static/warm-theme.css` を新規作成し、`base.html` で `app.css` の**後に**読み込む
- CSS Custom Properties（変数）でTailwindのデフォルト色を上書き
- セマンティックなコンポーネントスタイルを書き直す
- テンプレート側のクラス名は**最小限の変更**に留める（大規模置換を避ける）

### やらないこと
- `app.css`（ビルド済みバンドル）は触らない
- Tailwindのビルドを再構成しない
- JS側（Toast、Service Worker）は変更しない

---

## デザインコンセプト: "Warm Radio"

### 雰囲気の言語化
- 古いラジオ受信機のウッドパネル、真空管の温かい光
- 喫茶店でラジオを聴いている感じ
- デジタルだけど手作り感がある、人間が作ったUIっぽさ

### カラーパレット

```css
/* ライトモード */
--warm-bg:        #fdf6e3;   /* 羊皮紙クリーム */
--warm-bg-alt:    #f5ede0;   /* やや濃いクリーム（カードなど） */
--warm-surface:   #fffbf4;   /* 最も明るいサーフェス */
--warm-border:    #e0c9a6;   /* 温かみのあるボーダー */
--warm-text:      #3d2b1f;   /* 深い暖色ブラウン */
--warm-text-muted:#7a5c44;   /* ミュートテキスト */
--warm-accent:    #c8892a;   /* 琥珀色アクセント（メイン） */
--warm-accent-dk: #a06820;   /* アクセントhover */
--warm-red:       #c0392b;   /* 削除・エラー */
--warm-green:     #5a8a5a;   /* 成功・完了 */
--warm-nav-bg:    #3d2b1f;   /* ナビ背景（ダークブラウン） */
--warm-nav-text:  #f5e6c8;   /* ナビテキスト（薄いクリーム） */

/* ダークモード */
--warm-bg:        #1c1410;   /* ダークブラウン */
--warm-bg-alt:    #241a14;   /* やや明るいサーフェス */
--warm-surface:   #2e2018;   /* カード */
--warm-border:    #4a3020;   /* ボーダー */
--warm-text:      #f0e0c0;   /* クリームテキスト */
--warm-text-muted:#a08060;   /* ミュート */
--warm-accent:    #e0a040;   /* 明るめアンバー */
--warm-nav-bg:    #120e0a;   /* 最も深いナビ */
```

### タイポグラフィ
- フォント: `'Hiragino Mincho ProN', 'Yu Mincho', serif`（明朝体で温かみを）
  - ただし小さいUI要素は既存のゴシック体のまま
  - h1, h2, サイトタイトルのみ明朝体
- 行間: 少し広め（1.8）

### カード・コンポーネント
- 背景: `--warm-bg-alt`（クリーム）
- ボーダー: `1px solid --warm-border`（角を少し角ばらせる）
- 影: `0 2px 8px rgba(100, 60, 20, 0.12)`（青みのない暖色シャドウ）
- hover: `translateY(-2px)` + 影を少し強める
- border-radius: `8px`（丸すぎない）

### ナビゲーション
- 背景: ダークブラウン（現在の青紫グラデから変更）
- ロゴ: 明朝体 or ウッド感のある見た目
- リンクhover: クリーム色でアンダーライン

### ボタン
- プライマリ: 琥珀色（`--warm-accent`）、hover で少し暗く
- セカンダリ: クリーム+ブラウンボーダー
- 形状: `border-radius: 6px`（丸すぎない）

---

## 実装ステップ

### Step 1: `web/static/warm-theme.css` を新規作成

```css
/* CSS Custom Properties の定義 */
:root { ... }
[data-theme="dark"] { ... }  /* or prefers-color-scheme: dark */

/* body */
body { ... }

/* .card-base のオーバーライド */
.card-base { ... }

/* ナビゲーション */
nav.warm-nav { ... }

/* ボタン */
.btn-warm-primary { ... }
.btn-warm-secondary { ... }

/* フォーム */
input, textarea, select { ... }

/* タイポグラフィ */
h1, h2, h3 { font-family: 'Hiragino Mincho ProN', serif; }
```

### Step 2: `base.html` を修正

```html
<!-- 既存 -->
<link rel="stylesheet" href="/static/app.css">
<!-- 追加（これが既存スタイルを上書き） -->
<link rel="stylesheet" href="/static/warm-theme.css">
```

- `data-bs-theme` → `data-theme` に変更（Bootstrap依存を外す）
- nav の Tailwind グラデクラスを削除し、`.warm-nav` クラスに置換
- `<main class="main-content ...">` の Tailwind クラスを整理

### Step 3: テンプレートを順次修正

優先度順:

| ファイル | 変更内容 |
|---|---|
| `layouts/base.html` | nav色変更、フォント読み込み追加 |
| `home/top.html` | ヒーローセクション、機能カード |
| `radioprogram/detail.html` | メインのコンテンツページ |
| `post/list_all.html` | レビュー一覧 |
| `post/create.html` | フォーム系 |
| `auth/login.html` 等 | フォーム系 |
| その他ページ | 順次 |

---

## Tailwindクラスの扱い方針

Tailwindを完全に消すのは工数が大きいため、**見た目に直結するクラスのみ上書き**する。

```
残すもの（レイアウト用）:
  max-w-3xl, mx-auto, px-4, flex, grid, space-y-4, gap-4,
  hidden, block, w-full, overflow-hidden, truncate ...

上書きするもの（見た目用）:
  bg-white, bg-gray-*, text-gray-*, border-gray-*
  → CSS変数で色を差し替えることでクラスはそのままでも色が変わる

  ただしTailwindの bg-white はCSS変数を参照しないため
  → .card-base など主要コンポーネントは直接上書き
```

---

## ファイル変更一覧

### 新規作成
- `web/static/warm-theme.css` ← **メイン作業**

### 修正
- `web/templates/layouts/base.html`
  - `<link>` タグに warm-theme.css 追加
  - `data-bs-theme` → `data-theme` に変更
  - nav のグラデクラス削除 → `.warm-nav` に
  - テーマ切替スクリプト更新
  - フッター整理

- `web/templates/home/top.html`
  - ヒーローのグラデをウォームカラーに
  - カードデザインはCSS側で自動適用されるはず

- `web/templates/radioprogram/detail.html`（他ページも同様）
  - Tailwind の `bg-primary-*` → `bg-warm-accent` 的なクラスに変更
  - 主要ボタンを `.btn-warm-primary` に変更

---

## ダークモードの処理

現在 `data-bs-theme="dark/light"` を使っているが、Bootstrap依存なので変更する。

```js
// base.html のインラインスクリプトを修正
const theme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
document.documentElement.setAttribute('data-theme', theme);  // data-bs-theme → data-theme
```

```css
/* warm-theme.css */
@media (prefers-color-scheme: dark) {
  :root { /* ダーク変数 */ }
}
/* または */
[data-theme="dark"] { /* ダーク変数 */ }
```

---

## 注意事項・落とし穴

1. **`app.css` の Bootstrap の `!important`** が多いため、warm-theme.css では必要に応じて `!important` で上書きが必要な箇所がある

2. **Tailwind の `dark:` 修飾子**（例: `dark:text-white`）は `data-theme` 属性では動かない。Tailwindは `.dark` クラスか `prefers-color-scheme` で動く。現在どちらの設定かを確認が必要。
   - → テンプレートの `dark:*` クラスを `.warm-theme.css` の `[data-theme=dark]` セレクタで上書きすればOK

3. **`card-base` ユーティリティ**の定義場所を確認する（app.css のどこかにあるはず）。見つけてから上書き戦略を決める。

4. **テスト**: `go test ./...` は HTML変更に影響しないが、サーバーを起動してブラウザ確認が必須。

---

## 作業開始手順（次の会話で引き継ぐ場合）

```bash
# 1. プロジェクトルートを確認
cd /Users/takafumiotsuka/project/radio_review_go

# 2. card-base の定義を探す
grep -n "card-base" web/static/app.css | head -5

# 3. warm-theme.css を新規作成（以下のStep 1から開始）
touch web/static/warm-theme.css

# 4. base.html に追加
# <link rel="stylesheet" href="/static/warm-theme.css"> を app.css の後に挿入

# 5. サーバー起動して確認
go run cmd/server/main.go
```
