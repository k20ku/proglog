# Proglog

**Distributed Log Service implemetation**:

- based on "Distributed Services with Go" (Travis Jeffery)
- dependencies and development environment is adapted to current Go echosystems.

Travis Jeffery 著『Distributed Services with Go』をベースにした分散ログサービスの実装リポジトリ。
本書の内容をベースにしながら、依存ライブラリや開発環境は現在の Go エコシステムに合わせて見直している。

## Running

Example:

```bash
go run main.go 8080
```

### API

POST:

```bash
curl -X POST localhost:8080 -d \
'{"record":{"value":"TGV0J3MgR28gIzML"}}'
```

Response:

```json
{"offset":1}
```

GET:

```bash
curl -X GET localhost:8080 -d \
'{"record":{"offset":1}}'
```

Response:

```json
{"record":{"value":"TGV0J3MgR28gIzML","offset":0}}
```

## Motivation

このプロジェクトでは分散ログサービスの実装だけでなく、

- Context 管理
- Service Design
- mTLS
- gRPC
- Distributed Systems
- Test Driven Development

など、実運用を意識した Go サービス設計を学ぶことを目的としている。

## Current Status

- Chapter 1 完了
- HTTP API 実装済み
- `POST` / `GET` によるログ保存・取得確認済み

今後、

- gRPC
- mTLS
- Service Discovery
- Distributed Log
- Raft

などを実装予定。

## Improvements

本書の設計は非常にシンプルかつ洗練されているため、本体ロジックには極力手を加えていない。
一方で、保守状況や現在の Go エコシステムを考慮し、一部の周辺技術は置き換えている。

### 1. Gorilla Mux → Chi

本書では Gorilla Mux を利用している。しかし Gorilla Mux は一時 Archive された経緯があり、現在も活発な開発は行われていない。
そのため本プロジェクトでは Chi を採用している。

**採用理由**:

- 現在も継続的に保守されている
- 実利用例が多い
- `net/http` に忠実な設計
- 移行コストが小さい

### 2. CFSSL → Step CLI

本書ではサービス認証に CFSSL を利用している。

事前調査の結果、

- 設定管理が煩雑
- 開発が停滞気味

であることが分かったため、現在は Smallstep の Step CLI を利用している。

証明書形式は PEM のため、アプリケーション本体への影響はない。

### 3. Context and Service Lifecycle

Chapter 1 の段階ではシンプルなサーバー実装になっているが、起動処理は以下のように分離している。

```go
func run(ctx context.Context, l net.Listener) error
```

これは別プロジェクトで学習した Context 管理や Graceful Shutdown を将来的に導入しやすくするためである。

現時点では:

- signal handling 未実装
- graceful shutdown 未実装
- integration test 未実装

だが、本書の進行を優先するため一旦保留としている。

### 4. Buf

本書ではprotoファイルからの生成を `protoc` コマンドで行っている

しかしながら以下のような問題点に直面した

- 依存関係が多く，環境によって差異が出る
- Makefileの肥大化
- tools依存

そこで BufBuild を導入した．

## Notes

本書のコードは非常に読みやすく、設計も優れているため、本体ロジックについては原著を尊重している。

主な変更点は、

- 保守が止まった依存ライブラリの置き換え
- 開発環境の改善
- `Context` やテスト容易性を意識した構成への調整

に留めている。
