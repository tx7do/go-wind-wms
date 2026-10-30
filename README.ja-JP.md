<div align="center">

<img src="docs/brand/vortex-tile.svg" width="120" alt="GoWind WMS" />

# GoWind WMS

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

[English](./README.en-US.md) | [中文](./README.md) | **日本語**

</div>

---

倉庫・在庫・現場作業——小規模 3PL・EC・工場完成品倉庫の核心を、ひとつのシステムで閉じます。オーナー別の在庫分離、ロケーション単位の管理、入出庫移動（状態機械 + サーバー検証の書き戻し）、倉庫間転送（単一トランザクションで原子的実行）、冪等な在庫取消。データは一度入力すれば全体に行き渡ります。導入コンサルタントも、長い導入期間も不要です。

**GoWind WMS を 3 文で：**

- **導入コンサルタント不要** — 登録すればすぐ開始：テナント・部門・ロール・管理者は自動初期化、権限・承認・サイト内メッセージ・監査などの基盤機能をすべて内蔵
- **半日で使い始められる** — 管理コンソールとモバイル PDA を同梱。転送・棚卸・在庫照合のコアフローがそのまま動き、倉庫の現場はバーコードスキャンで、経営層は在庫ダッシュボードで
- **帳簿と現場は常に一致** — Odoo 流の stock.quant/picking/move モデリング：すべての在庫変動には move-line の実行記録が残り、数量とコストは同一トランザクション内で書き戻され、完全に追跡可能

開発者にも扱いやすい設計：API ファースト（RESTful / gRPC デュアルプロトコル、OpenAPI ドキュメント自動生成）、マイクロサービス、ネイティブ多言語対応——詳しくは下記の[テクノロジースタック](#テクノロジースタック)を参照してください。

## プロジェクトの状況

GoWind WMS は GoWind ERP の在庫モジュールと汎用基盤から派生しました（実績のあるマルチテナント / 権限 / 監査 / 承認レールのスケルトンをそのまま継承）。現在のマイルストーン M0 —— **スケルトン完成**：3 サービス構成のバックエンド（admin / app BFF + core）がビルド・テストともグリーン。在庫エンジン（quant / picking / move / lot、デュアルトラック仮想ロケーション、FEFO ロット自動分割控除、移動平均コスト）、転送・棚調整フロー、SaaS サブスクリプション割り当てガードは基盤から継承済み。ERP 業務モジュール（調達 / 販売 / 財務）は剥離済み。

> ロードマップ：M1 入庫（ASN → 受入 → 検品 → 納品方法）→ M2 出庫（ウェーブ → ピッキング → 梱包 → 引き渡し）→ M3 在庫運用（循環棚卸、フリーズ、スナップショット、エイジングレポート）→ M4 オーナー別課金 → M5 サードパーティ連携（送り状など）→ M6 デモサイト & 正式リリース。

## コア機能

### 組織 & 権限

| 機能 | 説明 |
|:---|:---|
| マルチテナント管理 | テナントの追加、有効化/無効化、データ分離。新規テナントの部門、ロール、管理者を自動初期化 |
| ユーザー管理 | ライフサイクル全体を管理、複数ロール/部署/役職のバインディング |
| ロール管理 | ロール & ロールグループ管理、メニュー/API/データ権限の設定 |
| 権限管理 | 権限グループ、メニューノード、権限ポイント、ポリシーエンジンによる認可、ボタンレベルの制御 |
| メニュー管理 | ディレクトリ/ページ/ボタンの 3 レベル設定、権限に基づく動的レンダリング |
| 部門 & 役職管理 | 複数レベルの部門ツリーと役職階層、ユーザーとの連携バインディング |
| 認証 & ログインポリシー | JWT の発行と検証、ログインポリシー設定、資格情報 & トークンキャッシュ管理 |

### システム & 運用

| 機能 | 説明 |
|:---|:---|
| 監査ログ | ログイン、操作、API 呼び出し、データアクセス、権限変更、ポリシー評価の全リンク監査 — 操作者、IP、パラメータ & 結果を記録 |
| ファイル管理 | ローカルまたはオブジェクトストレージへの統一アップロード/ダウンロード、プレビュー & グループ管理 |
| 辞書管理 | データ辞書カテゴリ & アイテム、連携する多言語翻訳 |
| サイト内メッセージ | 複数レベルのメッセージカテゴリ、対象配送 & 既読ステータス追跡、個人受信トレイ |
| タスクスケジューリング | Cron ジョブ管理、開始/一時停止/即時実行、実行履歴 & ログ |
| 多言語管理 | 言語管理、コンテンツ/メニュー/UI テキストの統一翻訳 |

### WMS 業務モジュール

| モジュール | 説明 |
|:---|:---|
| 倉庫 / ロケーション | 倉庫管理（受入ロケーション自動生成）、デュアルトラックロケーション体系：INTERNAL / SUPPLIER / CUSTOMER / INVENTORY_LOSS |
| 在庫エンジン | quant（ロケーション×製品ユニーク）+ move（状態機械）+ move-line（実行記録）+ lot（ロット/有効期限、FEFO 自動分割控除）、移動平均コスト |
| ピッキング / 転送 | picking を第一級ドキュメントとして（派生状態、確認→検証ライフサイクル）、倉庫間転送を単一アトミックトランザクションで、キャンセルと削除 |
| 棚卸 / 調整 | INVENTORY_ADJUSTMENT による過不足（同一単一方向、仮想ロケーション自動処理）、ソースコスト凍結 |
| 経営ダッシュボード | 倉庫数 / SKU 数 / 在庫総量 / 低在庫リスト / 30 日間の移動トレンド |
| サブスクリプション & 割当 | プランカタログ + テナント契約 + 期限切れ時リードのみ + 月次伝票/ユーザー数割当ガード |
| 承認レール（汎用） | 汎用承認スケルトン（多段フロー、自己承認禁止、承認結果をサイト内メッセージで通知）。将来の WMS ドキュメント（調整承認、棚差承認）の接続先 |
| モバイル業務機能 | バーコードスキャン、承認センター、経営ダッシュボード |


## テクノロジースタック

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Vue](https://img.shields.io/badge/Vue-3.x-4FC08D?logo=vue.js&logoColor=white)](https://vuejs.org/)
[![Flutter](https://img.shields.io/badge/Flutter-3.x-02569B?logo=flutter&logoColor=white)](https://flutter.dev/)
[![Kratos](https://img.shields.io/badge/Kratos-2.9-00ADD8?logo=go&logoColor=white)](https://go-kratos.dev/)

### バックエンド

| レイヤー | 技術 | 説明 |
|:---|:---|:---|
| 言語 | [Go 1.25+](https://go.dev/) | 高性能コンパイル言語 |
| フレームワーク | [go-kratos](https://go-kratos.dev/) | マイクロサービスフレームワーク |
| DI | 手書きアセンブリ (`wiring.go`) | DI フレームワークなし。`make register` が登録を補助 |
| ORM | [Ent](https://entgo.io/) | Go エンティティフレームワーク |
| データベース | [PostgreSQL](https://www.postgresql.org/) | リレーショナルデータベース |
| キャッシュ | [Redis](https://redis.io/) | インメモリデータストア |
| オブジェクトストレージ | [MinIO](https://min.io/) | S3 互換オブジェクトストレージ |
| サービスレジストリ | [Etcd](https://etcd.io/) | サービスディスカバリ & 設定管理 |
| トレーシング | [Jaeger](https://www.jaegertracing.io/) + [OpenTelemetry](https://opentelemetry.io/) | 分散オブザーバビリティ |
| API 定義 | [Protobuf](https://protobuf.dev/) + [buf.build](https://buf.build/) | コントラクトファースト API 設計 |

### 管理コンソールフロントエンド

| 技術 | 説明 |
|:---|:---|
| [Vue 3](https://vuejs.org/) | プログレッシブフロントエンドフレームワーク |
| [TypeScript](https://www.typescriptlang.org/) | 型安全開発 |
| [Ant Design Vue](https://antdv.com/) | エンタープライズ UI コンポーネントライブラリ |
| [Vben Admin](https://doc.vben.pro/) | 管理画面フレームワーク |

### モバイルフロントエンド

| バージョン | 技術スタック | 用途 |
|:---|:---|:---|
| Flutter | [Flutter](https://flutter.dev/) + [BLoC](https://bloclibrary.dev/) | クロスプラットフォームネイティブアプリ |

## プロジェクト構造

```
go-wind-erp/
├── backend/                        # バックエンドサービス
│   ├── api/                        # Protobuf API 定義 & 生成コード
│   │   ├── protos/                 # .proto ソースファイル
│   │   └── gen/                    # 生成コード (Go / Dart / OpenAPI)
│   ├── app/
│   │   ├── admin/service/          # 管理用 BFF (HTTP/gRPC)
│   │   ├── app/service/            # モバイル用 BFF (HTTP/gRPC)
│   │   └── core/service/           # コアサービス (ビジネスロジック + データ層)
│   ├── pkg/                        # 共通パッケージ (認可/暗号化/イベントバス/JWT/ミドルウェア/OSS...)
│   └── scripts/                    # デプロイスクリプト (Docker/環境設定)
├── frontend/
│   ├── admin/                      # 管理コンソールフロントエンド (Vue3 + Ant Design Vue + Vben Admin)
│   └── app/
│       └── flutter_app/            # モバイルアプリ (Flutter クロスプラットフォームネイティブ)
└── LICENSE
```

## クイックスタート

### 前提条件

- Go 1.25+
- Docker & Docker Compose
- Node.js 18+ & pnpm
- Flutter SDK
- buf (Protobuf ツールチェーン)

### 1. 依存サービスの起動

```bash
cd backend

# Windows
.\scripts\docker\libs_only.ps1

# Linux / macOS
./scripts/docker/libs_only.sh
```

### 2. バックエンドの起動

```bash
# 推奨：gow CLI を使用
gow run admin
gow run app
```

### 3. フロントエンドの起動

```bash
# 管理コンソール
cd frontend/admin
pnpm install
pnpm dev

# モバイルアプリ (Flutter)
cd frontend/app/flutter_app
flutter pub get
flutter run
```

### よく使うコマンド

```bash
cd backend

# Protobuf API コードの生成
make api

# OpenAPI ドキュメントの生成
make openapi

# 全コードの生成 (ent + api + openapi)
make gen

# 全サービスのビルド
make build

# テストの実行
make test
```

> 開発ワークフローの詳細は [バックエンドドキュメント](./backend/README.md) と [スクリプトガイド](./backend/scripts/WORKFLOWS_AND_BEST_PRACTICES.md) を参照してください。

## お問い合わせ

- WeChat 個人アカウント：`yang_lin_bo`（備考：`go-wind-erp`）

## コントリビューション

以下のような貢献を歓迎します：

- [Issue](../../issues) の提出：バグ報告や機能提案
- [Pull Request](../../pulls) の提出：修正や新機能の追加
- ドキュメントや翻訳の改善
- 利用経験の共有

## ライセンス

このプロジェクトは [MIT License](./LICENSE) の下で公開されています。

## 謝辞

[![JetBrains](https://resources.jetbrains.com/storage/products/company/brand/logos/jb_beam.svg)](https://jb.gg/OpenSource)

[JetBrains](https://jb.gg/OpenSource) から無料の GoLand & WebStorm オープンソースライセンスを提供していただき、感謝いたします。
