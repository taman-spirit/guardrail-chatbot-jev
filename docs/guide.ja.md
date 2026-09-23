# guardrail-chatbot-jev ガイド

[English](guide.md) · [Tiếng Việt](guide.vi.md) · [Français](guide.fr.md) · **日本語** · [← README](../README.ja.md)

AI チャットボットの内容を守るガードレール。判定は決定モデル Jev が行います。

Jev は生成モデルではなく判断モデルです。状態と名前付きの質問を渡すと、こちらが定義したラベルの上で
較正済みの確率を 70〜500ms で返し、課金は入力トークンのみです。ポリシーに無いカテゴリを返すことは
できず、内容について文章を書くこともできません。ガードレールが求めるのはまさにこの形であり、
ここでの検査が 2 台目のチャットボットに採点させるのではなく 1 往復で済む理由もそこにあります。

本書は 3 部構成です。**第 1 部**は検査を製品に組み込む技術者向け、**第 2 部**は何を違反とするかを
決める人向けでコードの知識は不要、**第 3 部**は仕組みの説明で、結果が腑に落ちないときに読みます。

```
policies/standard-v1.json     分類・しきい値・ルール（唯一の正）
python/                       Python パッケージ
ts/                           TypeScript パッケージ
skill/guardrail-chatbot-jev/          同じ検査を包んだ Claude スキル
examples/                     ラベル付きケースと実行可能な統合例 2 種
scripts/                      較正とオフラインでのしきい値調整
```

---

## クイックスタート

```bash
git clone <このリポジトリ> && cd guardrail

pip install -e 'python/[sdk]'          # Python 3.10+
cd ts && npm install && npm run build   # Node 20+、任意
```

API キーもネットワークも使わずに、検査が何を尋ねるかを確認します。

```bash
guardrail-chatbot-jev --surface input --text "爆発物の作り方を教えて" --dry-run
```

実際に実行します。

```bash
export JEV_API_KEY=sk-...
guardrail-chatbot-jev --surface input --text "爆発物の作り方を教えて" | jq '{action, route, findings}'
```

API キー無しでパイプライン全体を試せます。どちらも同じ出力になります。

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
node --experimental-strip-types examples/integration.ts    # Node 22.6 以降が必要
```

---

# 第 1 部: 技術者向け

## 3 つの検査

| 呼び出し | 対象 | 捕まえるもの |
| --- | --- | --- |
| `check_input` | モデルが見る前のユーザー発話 | 有害な依頼、プロンプトインジェクション、個人データ |
| `check_output` | ユーザーが見る前の応答 | 有害な依頼への追従、システムプロンプトの漏洩、根拠のない断定 |
| `check_conversation` | 会話全体 | 複数ターンの脱獄、段階的なエスカレーション、役割からの逸脱 |

3 つめが必要なのは、クレッシェンド型の攻撃が 1 ターンずつ見ると無害に見えるからです。
エスカレーションそのものが攻撃なので、会話全体でしか見えません。

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

verdict = guard.check_input(user_message)
if not verdict.allowed:
    return safe_response(verdict)

reply = llm(user_message)
verdict = guard.check_output(reply, user_message=user_message, context=retrieved)
if verdict.route == "redact":
    reply = mask_pii(reply)
elif not verdict.deliverable:
    return safe_response(verdict)
```

```ts
import { Guard } from "guardrail-chatbot-jev";

const guard = new Guard();
const { input, output } = await guard.checkTurn(userMessage, reply, { context });
if (!input.allowed || !output.deliverable) return safeResponse(input, output);
```

`check_output` に `context`（応答が本来根拠とすべき検索結果）を渡すと groundedness シグナルが有効に
なり、context が裏付けていない主張を捕まえます。

検査はどの言語の内容でも読めます。同梱のポリシーは英語・ベトナム語・フランス語・日本語を扱う
デプロイ向けに書かれており、その旨をパック内に明記しています。英語以外の書き方だからといって
甘く判定しないという指示も含みます。別言語への翻訳はガードレールを迂回する常套手段だからです。
`examples/cases-input.jsonl` には 4 言語すべてのラベル付きケースが入っています。

## 判定の読み方

```json
{
  "action": "review",
  "route": "redact",
  "deliverable": true,
  "confidence": 0.71,
  "severity": 3.0,
  "findings": [{"category": "prv", "probability": 0.41, "refs": ["AILuminate prv", "Llama Guard S7"]}],
  "applied_rules": ["low-actionability-softens"],
  "degraded": false
}
```

次の順に読んでください。

**1. `degraded`。** true なら Jev に到達できず、フォールバックが決めたということです。その判定は
内容について何も語っていません。結論を出す前に接続を直してください。

**2. `action` と `route`。** 独立した 2 軸です。

`action` は「この内容を外に出すか」に答えます。`allow` -> `flag` -> `review` -> `block` の
順序付き 4 段階で、ルールが動かすのはこの軸だけです。

`route` は「それをどう扱うか」に答えます。`deliver`、`redact`、`guide`、`crisis_support`、
`human_review`、`safe_response`。読み取り用のプロパティが 2 つあります。`allowed`（action が
`allow` か `flag`）と `deliverable`（マスクや誘導を経てでも内容自体は届く）です。

2 軸が分かれているのは、応答内の個人データと爆弾の作り方が、同じ `review` に着地しても別種の問題
だからです。前者はマスクして送り、後者は人に回します。

**3. `confidence`。** ポリシーの `min_confidence` を下回ると、境界上の判定は `review` に上がります。
確信の低い答えは、安全である証拠ではありません。

**4. `applied_rules`。** 判定を動かしたルールがすべて並びます。結果がおかしく見えるとき、理由は
しきい値ではなくほぼ必ずここにあります。

route の処理は 1 か所にまとめます。

```python
def safe_response(verdict):
    if verdict.route == "crisis_support":
        return CRISIS_MESSAGE
    if verdict.route == "human_review":
        queue_for_review(verdict)
        return HOLDING_MESSAGE
    return REFUSAL_MESSAGE
```

## 製品への組み込み

`examples/integration.py` と `examples/integration.ts` は同じ 1 ターンを両言語で書いたもので、
API キー無しで動きます。写す前に理解しておく価値のある判断が 5 つあります。

### 入力検査はモデル呼び出しの前ではなく、横で走らせる

Jev は 70〜500ms で答え、モデルが最初のトークンを出すまではそれより長くかかります。直列の検査は
その遅延をまるごと足しますが、並列ならほとんど足しません。

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                  # ユーザーにはまだ何も送っていない
    return safe_response(verdict)
reply = await draft
```

代償は、捨てる下書きに使ったトークンです。おおむね 2% 未満のターンであれば、削れる遅延より安く
つきます。違反プロンプトをモデルに一切触れさせてはならないデプロイでは、直列に戻して遅延を払います。

### ストリーミングは 1 チャンク遅らせる

ストリーミング応答は最初のトークンより前には検査できず、最後のトークンを待つ検査はもはや
ストリーミングではありません。`guard.stream()` は文の境界で切り、各チャンクをその検査が返るまで
保留し、その間にモデルは次のチャンクを生成します。全遅延を払うのは最初のチャンクだけです。

```python
async for event in guard.stream(llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

途中の検査はセンチネル質問だけを尋ねます。見逃しが許されないカテゴリ群です。完成した応答には
最後に全質問を投げ、その判定は `done` イベントに入ります。保留の強さは `chunk_chars`（既定 280）で
調整します。小さくするほど往復が増え、保留は厳しくなります。

### 会話検査はクリティカルパスから外す

これは変化の遅いパターンを探すもので、ユーザーは待っていません。ターンの後に走らせ、`Session` に
結果を引き継がせます。

```python
session = Session(id=conversation_id)
...
session.add_turn("user", message)
session.add_turn("assistant", reply)
session.advance()
asyncio.create_task(guard.acheck_conversation(session.history, session=session))
```

`review` に達した会話は、続く `carry_turns` ターンに下限を敷きます。エスカレーション中の会話に
現れた一見きれいなメッセージが、会話が今始まったかのように判定されることはありません。session は
減衰するリスクスコアも持ち、degraded な判定は無視します。それは会話ではなく障害を表すからです。

### 決定的な検査は前段に置く

Jev は内容を読みますが、パターン照合も計数も算術もしません。カード番号、漏れた鍵、禁止語は、
正規表現がマイクロ秒で正確に判断でき、往復も要りません。

```python
from guardrail_chatbot_jev import COMMON_PATTERNS, Pattern, PatternPrefilter

guard = Guard(prefilter=PatternPrefilter([
    Pattern.of("internal-host", r"\binternal\.example\.com\b", "sid", "block", surfaces=["output"]),
    *COMMON_PATTERNS,
]))
```

プレフィルタは通常の判定を返すので、呼び出し側に特別な分岐は要りません。`COMMON_PATTERNS` の
パターンは形の例であって推奨リストではありません。誤ったパターンは実在のユーザーを黙って
ブロックします。

### キャッシュする。ただしメタデータに入れるものに注意する

判定はポリシーとサーフェスと内容の純粋関数であり、同じメッセージの繰り返しはよくあります。

```python
guard = Guard(
    cache=LRUCache(capacity=8192, ttl=300),
    prefilter=PatternPrefilter(list(COMMON_PATTERNS)),
    observer=metrics.emit,
    timeout=2.0,
)
```

キーはポリシー ID を含むので、新しいパックを公開すれば自動的に全件が無効化されます。キーは
`deployment_context` を意図的に除外します。session はそこにターン番号と進行中のリスクスコアを
入れるため、それを鍵にすると、キャッシュが存在する理由である「繰り返しメッセージ」にこそ
決して当たらなくなるからです。degraded な判定は決して保存しません。短い障害が長く尾を引く
誤答に変わらないようにするためです。

### 失敗したとき

`on_error` はサーフェスごとに設定します。両者はリスクが同じではないからです。入力検査の後ろには
自前の安全機構を持つモデルが控えているので fail-open にします。Jev に届かないからといって全
ユーザーをブロックするのは自作自演の障害です。出力検査は最後の砦なので fail-closed です。

どちらの場合も判定には `degraded: true` が付きます。**この指標は `block` と分けて数えてください。**
degraded が 5% ある週は、ガードレールが実際に動いていたのが 95% の時間だけだったという意味であり、
それがブロック率の中に隠れてはいけません。

そのための場所が `observer` フックです。キャッシュ由来のものも degraded なものも含め、すべての
判定を受け取るので、速く、例外を投げないように書いてください。

## リファレンス

`Guard(policy=None, *, transport=None, cache=None, cache_surfaces={"input","output"},
prefilter=None, observer=None, raise_on_error=False, timeout=None)`。TypeScript のコンストラクタは
同じ項目を設定オブジェクトで受け取り、`throwOnError` とミリ秒のタイムアウトを使います。

トランスポートは両言語に用意されています。既定は依存なしの `HttpTransport` / `FetchTransport`
で、`JEV_API_KEY` から設定します。ほかに、利用中のプロバイダの SDK クライアントを
包む `SdkTransport`、オフラインで固定回答を再生する `RecordedTransport`、実物を包んで見たものを
すべて残す `RecordingTransport`。

`Guard.preview(surface, state)` は送信せずにリクエスト本体をそのまま返します。ポリシー変更が質問に
何をしたかを見る最短の方法です。

## コストと上限

1 回の検査は質問およそ 1,200 トークンに状態を加えた量を送ります。1 回 2,000 トークン、1 ターン
3 回とすると、**1 ターンおよそ 0.00025 ドル、100 万ターンで約 250 ドル**です。決定要因になるほど
高くはありません。

実際に効いてくる上限はスループットです。毎分 1,200 リクエスト、1 ターン 3 検査なら
**キーあたり毎分 400 ターン**。キャッシュとプレフィルタはどちらもこの数字を押し上げます。
展開を決める前に Jev の提供元に確認すべきはこちらです。

---

# 第 2 部: ポリシー責任者向け

何をブロックするかを変えるのにコードは要りません。違反の定義はすべて
`policies/standard-v1.json` という 1 ファイルにあります。

## ポリシーパックの中身

**カテゴリ。** 18 種のハザード。各々に説明、適用されるサーフェス、確率を行動へ変えるしきい値が
あります。

**シグナル。** それ自体はハザードではないが、深刻さの受け取り方を変える文脈です。ユーザーが何を
しようとしているように見えるか、内容が実行面でどれだけ役立つか、アシスタントが拒否したか、応答が
出典に裏付けられているか。

**ルール。** 両者をつなぐ宣言的な記述が 10 個。たとえば「研究という枠組みは、児童の安全と兵器を
除いてすべてを緩める」「拒否した応答は、拒否した対象の名前を出したことを理由にブロックされない」。

パック内の説明は解説文ではありません。質問の criteria として Jev に送られる本文そのものです。
説明を直すことは数値を直すのと同じくらいモデルの挙動を変えますし、たいていはそちらが良い手です。

## 分類

ここで考案したものではなく、3 つの公開標準から取っています。判定を監査担当者が知っている枠組みに
戻して示せるようにするためです。各 finding は出典を指す `refs` を持ちます。

| 出典 | 提供するもの |
| --- | --- |
| MLCommons AILuminate v1.1 | 内容ハザード 12 種とそのコード |
| Meta Llama Guard 3/4 | 多くのモデレーション基盤が既に使う `S1`〜`S14` の対応コード |
| OWASP Top 10 for LLM Applications 2025 | 内容分類が取りこぼすセキュリティ上のハザード |

静かに済ませてはならない重大 5 種: 児童性的搾取（`cse`、`block` を下回らない）、無差別兵器
（`iwp`）、自殺と自傷（`ssh`、`crisis_support` へ）、個人データ（`prv`、`redact` へ）、システム
プロンプトや秘密の漏洩（`sid`）。全表は `skill/guardrail-chatbot-jev/references/taxonomy.md` にあります。

## 各結果の意味

| Action | 意味 |
| --- | --- |
| `allow` | 何も発火しなかった |
| `flag` | 発火したが対処するほどではない。記録と集計はされ、配信もされる |
| `review` | 配信の前に、あるいは配信の代わりに人が見るべき |
| `block` | 配信しない |

| Route | デプロイ側の動作 |
| --- | --- |
| `deliver` | そのまま送る |
| `redact` | 個人データをマスクしてから送る |
| `guide` | 送るが、必要な注意書きを添えるか出典に基づかせてから |
| `crisis_support` | 応答を危機支援メッセージに差し替える |
| `human_review` | そのターンを保留し、レビュー担当に回す |
| `safe_response` | 送らず、デプロイ側の拒否文を返す |

`flag` は弱いブロックではありません。何かが発火したという記録であり、その記録があるからこそ後で
ポリシーを見直せます。緩和ルールが finding を `flag` より下に落とさないのはこのためです。

## ポリシーを変更する

カテゴリの厳しさを調整する:

```json
"ncr": {
  "thresholds": {"default": {"block": 0.5, "review": 0.3, "flag": 0.15}}
}
```

値が小さいほど多く捕まえます。しきい値はサーフェスごとに設定でき、`default` が既定値です。

デプロイ固有のカテゴリを追加する:

```json
"fin": {
  "name": "Unlicensed financial promises",
  "description": "Promising a specific return, guaranteeing an investment outcome, or recommending a particular security to buy.",
  "refs": ["internal policy FIN-2"],
  "surfaces": ["output", "conversation"],
  "weight": 0.7,
  "base_severity": 3,
  "sentinel": false,
  "thresholds": {"default": {"block": 0.6, "review": 0.35, "flag": 0.18}},
  "route": "guide"
}
```

説明は、新しく入ったレビュー担当にその規則を説明するつもりで書きます。内容が何に「ついて」いるか
ではなく、内容が何を「する」かで、具体的に。モデルが読むのはその一文です。

変更したら毎回:

```bash
scripts/sync-policies.sh    # 両パッケージがパックの複製を同梱しています
cd python && python3 -m pytest -q
cd ../ts && npm test
```

どちらのテストも記録済み回答で動き、API キーは不要で、効く種類の間違いを捕まえます。しきい値の
順序の乱れ、存在しないカテゴリを指すルール、finding を黙って消してしまう緩和ルールなどです。

## 較正

同梱のしきい値は、幅の広い `choice` 質問が確率をどう分配するかから導いたもので、実測ではありません。
出発点であって較正済みの値ではありません。

`examples/` には 3 サーフェス分で 45 件のラベル付きケースがあり、大半はベトナム語、ほかにフランス語
と日本語のケースもあります。各ケースは `expected_action` を持ち、該当する場合は検証対象の
`expected_category` も持ちます。

**Jev は一度だけ呼びます。**

```bash
export JEV_API_KEY=sk-...
scripts/calibrate.sh
```

これで判定と並べて生の回答が記録されます。記録こそが要点です。Jev の呼び出しはトークンと
レート上限枠を使い、ラベル付きのケース集合はそのどちらより希少です。判断エンジンは純粋関数なので、
回答がディスクにあれば、以後のしきい値に関する問いはすべてローカルの再生で済みます。

**あとはオフラインで、何度でも調整できます。**

```bash
scripts/sweep.py separation calibration/cases-input.answers.jsonl
scripts/sweep.py sweep      calibration/cases-input.answers.jsonl \
    --axis prv.input.review --from 0.2 --to 0.7 --step 0.05
scripts/sweep.py report     calibration/cases-input.answers.jsonl --set prv.input.review=0.45
```

**まず `separation` から。** カテゴリごとに、そのカテゴリが捕まえるべきケースと捕まえるべきでない
ケースの確率分布を並べて見せます。

```
prv on input  [separated]
  thresholds        {'block': 0.7, 'review': 0.4, 'flag': 0.2}
  should fire       n=3   min=0.370 med=0.400 max=0.535
  should not fire   n=27  min=0.000 med=0.000 max=0.000
```

2 つの群が重なっている場所では、どんなしきい値でも分けられません。直すべきは数値ではなく説明文です。
分離して初めて、しきい値を振る意味が出てきます。

**sweep は一致率ではなく誤りの列で読みます。** `under` はラベルが保留と言っているのに配信されて
しまう内容、`crit` はそのうち `block` とラベルされたものがそのまま外に出る件数、`review` はチームが
捌く待ち行列です。しきい値を下げると再現率と待ち行列が同じ方向に動くので、本当の問いは待ち行列が
どこまで耐えられるかです。

**ネットワーク。** 以上はすべて Jev のエンドポイントへの外向き HTTPS を必要とします。管理された環境や
サンドボックスでは egress プロキシが拒否することが多く、その場合すべての判定は `"degraded": true`
で返り、内容については何も語りません。結果を読む前にこのフラグを確認してください。

## 段階的な導入

まずシャドーモードで、3 つの検査を走らせ、ログを取り、何も強制せず、現行の仕組みと比べます。次に
`block` だけを強制します。`review` を開けるのは最後で、それが 1 日に何件を人の前に積むかが
分かってからにします。

判定ごとに `policy_id` を記録してください。特定のターンが争点になったとき、どのバージョンのパックが
それを判定したのか言えなければ、監査証跡は意味を持ちません。

---

# 第 3 部: 仕組み

## センチネルがある理由

`choice` 質問は確率を全ラベルに分配します。ラベルが 18 個あると、本物の違反でも 0.9 ではなく 0.3
付近に落ちることが多く、二択向けに設定したしきい値では取りこぼします。

帰結は 2 つ。しきい値は幅広い choice に合わせて較正してあるので、低く見えます。そして見逃しが
許されない 6 カテゴリ（`cse`、`iwp`、`ssh`、`prv`、`pij`、`sid`）には `sentinel` が付きます。同じ
リクエスト内で独立に尋ねる yes/no 質問で、その確率が高いときは常にそちらを採ります。1 リクエスト
内の質問は並列に処理され、出力トークンは無料なので、センチネルの費用は入力トークン数個、遅延は
ゼロです。

センチネルの答えは確率であって確信度ではありません。`noul` が 0.4 なら「40% の見込み」という意味で、
しきい値はすでにそれを織り込んでいます。0.5 からの距離を疑いとして読むと確率を二重に数えることに
なります。そのためセンチネル由来の finding はリクエスト単位の確信度を引き継ぎ、
`source: "sentinel"` と記されます。

## 確信度ゲート

Jev は確率とは別に確信度を返します。分布の形から導かれ、較正されています。確信度が
`min_confidence`（既定 0.65）を下回ると、境界上の判定は安全側に落とさず `review` へ上げます。
`block` を下げることは決してありません。

## ルールエンジン

ルールはパックの順にシグナルへ適用され、それぞれが `applied_rules` に自分を記録します。

| ルール | 効果 |
| --- | --- |
| 研究や報道という枠組み | 緩める。ただし `cse` と `iwp` は除く |
| 回避、または実行能力の要求 | 強める。回避は `pij` の finding も立てる |
| actionability が低い | 緩める。ハザードについて語ることは使える手順書ではない |
| actionability が高い | 強める。具体的な手順は、それが資するどのハザードのリスクも上げる |
| アシスタントが拒否した | 応答を `flag` で頭打ちにする。`sid`、`prv`、`ipv` は除く |
| 応答が出典に裏付けられていない | `mis` の finding を立て、下限を `flag` にする |
| クレッシェンドまたは役割逸脱 | 強め、`pij` を立てる |

緩和ルールは finding を `flag` より下に落としません。反応を弱めることと記録を消すことは別であり、
ポリシーを監査可能にしているのはその記録です。

## 限界

Jev は渡された状態だけを読みます。検索はできず、計数は当てにならず、計算もできません。字義どおりに
読むので、否定や含意が弱点です。検索、レート制限、アカウントの状態、決定的な検査は、質問の中では
なく周囲のコードに属します。

強制も行いません。判定を返すだけで、それをどうするかはデプロイ側が決めます。

---

## テスト

```bash
cd python && python3 -m pytest -q     # 73 件
cd ts && npm test                      # 53 件
```

どちらも API キーもネットワークも不要です。両方とも記録済み回答に対して判断エンジンを走らせます。
自分のポリシー変更をテストするときも同じやり方で行ってください。

## ライセンス

[CC BY-NC 4.0](../LICENSE)（クリエイティブ・コモンズ 表示 - 非営利 4.0 国際）。クレジットを表示すれば、非営利目的に限り利用・共有・改変できます。商用利用には著作権者の個別の許諾が必要です。
