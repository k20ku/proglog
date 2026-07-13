# internal/log

ストア／インデックス／セグメントを積み重ねた永続コミットログの実装。
設計は Kafka を参考にしている（`segment` = Kafka の `LogSegment`、`index` は
Kafka の OffsetIndex や LevelDB のメタヘッダを参考）。

前提（不変条件）:

- **store が source of truth**。`[len][bytes]` のフレーミングで自己記述的なので、
  index はいつでも store から決定論的に再構築できる。
- **index は正しさのためではなく速さのためのキャッシュ**。
- roll 時にシール直前の旧セグメントが sync されるため、
  不整合になりうるのは基本的に **末尾（active）セグメントだけ**。

## 現状の問題点（クラッシュ復旧の穴）

### 1. サイズ喪失問題（不整合終了で毎回起きる）

`index.size` が論理エントリ数ではなく**ファイルサイズ由来**になっている
（`index.go` の `idx.size = uint64(fi.Size())`）。一方 `newIndex` は open 直後に
`os.Truncate(name, MaxIndexBytes)` で mmap 用に最大長まで**伸ばす**。

正常な `Close` は `i.file.Truncate(i.size)` で実サイズまで縮めて閉じるので次回は
正しいが、**電源断などで `Close` が走らないとファイルは `MaxIndexBytes` のまま残る**。
その結果、再起動時に:

- `idx.size = MaxIndexBytes` になり、`newSegment` の
  `if s.index.size == 0 && s.store.size > 0` による rebuild 分岐が**発火しない**。
- `index.Read(-1)` が末尾スロット（実際はゼロ埋め領域）を読み、`(off,pos)=(0,0)`
  を返す → `nextOffset` が壊れる。
- さらに `IsFull()` が true になり、log は次の Append で誤ったオフセットの
  新セグメントへ roll してしまう。

つまり「index が torn write で壊れた」以前に、**クリーンでない終了はすべて
このパスに落ちる**。ファイルサイズから論理エントリ数を復元しようとしているのが
根本原因。

### 2. 本当の破損（torn write / bit rot）の検知が無い

エントリは在るが中身がゴミ、というケースを検知できない。
なお `(off,pos)=(0,0)` は「先頭レコード」を指す正当なエントリでもあるため、
**ゼロ埋めスキャンで末尾を探すことはできない**（空スロットと区別できない）。
検知するなら次のいずれかが要る:

- **store とのクロス検証**（安い）: `index[i].pos` へ store を seek し、そこの
  レコードの decode 済み offset が `baseOffset+i` と一致するか確認する。
  不一致点が復旧開始点になる。
- **エントリ毎 CRC**（LevelDB paranoid_checks 流）: bit rot まで見たい場合。
  ただし store 側にも CRC が必要になり範囲が広がる。

### 3. Append の store→index 間クラッシュで orphan レコードが残る

`segment.Append` は store を書いてから index を書く。両者の間でクラッシュすると、
index エントリを持たない store レコードが残る（`segment.go` の WARNING コメント参照）。
未 ack の書き込みなので失われて構わないが、リカバリ時に store をどこまで信じるか
（末尾の壊れたレコードで truncate する）を決める必要がある。

## 想定している復旧方針（未実装・メモ）

Kafka の recovery point 方式:

1. `index.size` をファイルサイズから独立させ、pre-grow の影響を受けないようにする。
2. 正常 `Close` 時に clean マーカーを fsync。
3. 起動時、マーカー有り → index をそのまま信頼（高速パス）。
   マーカー無し → 末尾セグメントだけ store をスキャンして index を再構築し、
   途中で壊れたレコードに当たったら store をそこで truncate。
4. 破損検知が要るなら recover に store クロス検証を組み込む（CRC は後回し可）。
