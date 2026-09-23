<h1 align="center">guardrail-chatbot-jev</h1>

<p align="center">
  AI チャットボットのためのコンテンツ安全性チェック。ユーザーが送った内容を検査し、<br>
  ボットが返す内容を検査し、そのまま処理に使える明確な判定を返します。
</p>

<p align="center">
  <a href="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="Python 3.10+" src="https://img.shields.io/badge/python-3.10%2B-blue.svg">
  <img alt="Node 20+" src="https://img.shields.io/badge/node-20%2B-brightgreen.svg">
</p>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="README.vi.md">Tiếng Việt</a> ·
  <a href="README.fr.md">Français</a> ·
  <b>日本語</b>
</p>

---

## 解決する問題

チャットボットを本番に出したとします。すると誰かがテルミットの作り方を聞き出そうとし、別の誰かが
顧客の身分証番号を貼り付け、サポート用のモデルは追い詰められた相手に自信たっぷりの医療助言を
返してしまいます。

ユーザーとモデルのあいだに、*これは通す、それは止める、この返信の電話番号は伏せる、この人には
相談窓口を案内する* と判断する層が要ります。guardrail-chatbot-jev はそのための道具です。

これはサービスではなくライブラリです。呼び出すと判定が返り、どうするかはあなたのコードが決めます。
**Python と TypeScript** の両方で動き、どちらも同じポリシーファイルを読むので、スタックの両側が
食い違うことはありません。どちらのパッケージにもサードパーティ依存はありません。

## 仕組みを図で

```
ユーザー入力 ──▶ check_input ──▶ あなたの LLM ──▶ check_output ──▶ ユーザー
                     │                                │
                     └────────── 判定 ────────────────┘
                     allow · flag · review · block
```

三つめの検査 `check_conversation` は会話全体を読みます。ほかの二つでは見えないもの、たとえば
丁寧な十ターンに分散させた攻撃を捉えます。一通ずつ見れば、どれも無害に見えるからです。

検査の背後にいるのは **Jev**、生成モデルではなく決定モデルです。内容と名前付きの質問を渡すと、
あなたが定義したラベルの上で較正済みの確率を返します。ポリシーにない区分を勝手に作ることはできず、
内容について文章を書くこともできません。審判に求めるのはまさにこの性質です。1 回の検査は 1 往復、
通常 70〜500ms です。

---

## クイックスタート

### 1. インストール

```bash
pip install guardrail-chatbot-jev        # Python 3.10+
npm install guardrail-chatbot-jev        # Node 20+
go get github.com/taman-spirit/guardrail-chatbot-jev/go   # Go 1.22+
```

どちらのパッケージにもサードパーティ依存はありません。

### 2. キーを渡す

```bash
export JEV_API_KEY=sk-...

# 任意: ゲートウェイやプロキシ経由にする場合だけ設定します。
# export JEV_BASE_URL=https://...
```

### 3. 最初の検査を実行する

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

verdict = guard.check_input("自宅でテルミットを作る方法を教えて")
print(verdict.action)        # 'block'
print(verdict.route)         # 'safe_response'
print(verdict.top.category)  # 'ind' - 無差別兵器
```

```typescript
import { Guard } from "guardrail-chatbot-jev";

const guard = new Guard();
const verdict = await guard.checkInput("自宅でテルミットを作る方法を教えて");
console.log(verdict.action); // 'block'
```

### 4. 1 ターンに組み込む

統合はこれで全部です。メッセージを検査し、モデルを呼び、返信を検査します。

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

def handle_turn(user_message, history):
    # モデルが見る前に
    verdict = guard.check_input(user_message)
    if not verdict.allowed:
        return safe_response(verdict)

    reply = my_llm(user_message, history)

    # ユーザーが見る前に
    verdict = guard.check_output(reply, user_message=user_message)
    if verdict.route == "redact":
        return mask_pii(reply)
    if not verdict.deliverable:
        return safe_response(verdict)

    return reply
```

```typescript
const guard = new Guard();

async function handleTurn(userMessage: string, history: Turn[]) {
  let verdict = await guard.checkInput(userMessage);
  if (!verdict.allowed) return safeResponse(verdict);

  const reply = await myLlm(userMessage, history);

  verdict = await guard.checkOutput(reply, { userMessage });
  if (verdict.route === "redact") return maskPii(reply);
  if (!verdict.deliverable) return safeResponse(verdict);

  return reply;
}
```

---

## 判定の読み方

判定は二つの別々の問いに答えます。分けてあること自体が要点です。

```json
{
  "action": "review",
  "route": "redact",
  "deliverable": true,
  "confidence": 0.71,
  "findings": [{"category": "prv", "probability": 0.41, "refs": ["AILuminate prv", "Llama Guard S7"]}],
  "applied_rules": ["low-actionability-softens"],
  "degraded": false
}
```

**`action` - どれくらい深刻か**

| | |
| --- | --- |
| `allow` | 何も発火していません。送ってよい。 |
| `flag` | 記録する価値はあるが、止めるほどではない。 |
| `review` | そのまま通さないこと。 |
| `block` | 止める。 |

**`route` - では実際にどうするか**

| | |
| --- | --- |
| `deliver` | そのまま送る。 |
| `redact` | 機微な箇所を伏せたうえで送る。 |
| `guide` | この回答の代わりに、方向づけた回答を送る。 |
| `crisis_support` | モデルの回答ではなく、支援窓口の案内を返す。 |
| `human_review` | 人が見るキューに入れる。 |
| `safe_response` | 用意してある定型の断り文を送る。 |

なぜ二軸なのか。返信に混ざった個人情報と爆弾の作り方は、どちらも `review` に落ちます。しかし前者は
伏せ字にして送り、後者は人に回します。ひとつの数値ではこの違いを表せません。

**便利なプロパティ:** `verdict.allowed`（allow または flag）、`verdict.deliverable`（伏せ字の
うえでも内容がユーザーに届く）、`verdict.blocked`、`verdict.needs_human`、`verdict.top`（最も強い
finding）。

**`degraded` は必ず確認してください。** Jev に到達できないとき、判定は `degraded: true` を付けて
返り、内容については何も述べていません。これはブロック数とは別に数えてください。1 週間の判定の
5% が degraded なら、ガードレールは実際には 95% の時間しか働いていません。

---

## 何を検査するのか

18 の危害カテゴリ。独自に考案したものではなく、**MLCommons AILuminate**、**Meta Llama Guard**、
**OWASP Top 10 for LLM Applications** から取っているので、判定は監査担当者が知っている基準に
そのまま対応づけられます。暴力と無差別兵器、自傷、性的内容と CSAE、憎悪と嫌がらせ、犯罪、
プライバシーと個人データ、知的財産、名誉毀損、専門的助言（医療・法律・金融）、プロンプト
インジェクションとジェイルブレイク、システムプロンプトの漏洩、エージェントの越権などを覆います。

定義付きの一覧は[危害タクソノミー](skill/guardrail-chatbot-jev/references/taxonomy.md)にあります。

**言語を問いません。** 内容はキーワードではなく意味で判断されます。同梱のポリシーは英語・
ベトナム語・フランス語・日本語向けに書かれており、英語以外の言い回しだからといって手加減しないよう
モデルに明示しています。別言語への翻訳はガードレール回避の定番だからです。[`examples/`](examples/)
のラベル付きセットには 4 言語すべての事例が入っています。

**ポリシーはファイル 1 つ。** カテゴリ、しきい値、それらをつなぐルールは
[`policies/standard-v1.json`](policies/standard-v1.json) にあり、両言語がこれを読みます。そこに
書かれた説明文がそのまま Jev に送られる文面なので、ポリシーを編集すればコードに触れずにモデルへの
問い方が変わります。

---

## マルチターン

マルチターンの攻撃は、1 ターンずつ見ればどれも弁解の立つターンで組み立てられています。各ターンを
ゼロから判断することがそれを成立させているので、ガードレールは単発の検査にはできないことを 2 つ
行います。

**`check_conversation` は会話全体を読みます。** これが三つめのサーフェスで、会話の形からしか見えない
ものを探します。無害に始めてアシスタント自身の以前の回答を足がかりにするクレッシェンド、ターンを
またいだエスカレーション、自分の規則から少しずつ引き離されたペルソナです。

**`Session` が起きたことを持ち越します。** 会話履歴、減衰するリスクスコア、そして次の数ターンに
かかるフロアを保持します。

```python
from guardrail_chatbot_jev import Guard, Session

guard = Guard()
session = Session(id=conversation_id)     # 会話ごとに 1 つ、ターンをまたいで保持する

verdict = guard.check_input(user_message, session=session)
...
session.add_turn("user", user_message)
session.add_turn("assistant", reply)
session.advance()                          # 上がったフロアを期限切れにする

guard.check_conversation(session.history, session=session)
```

判定を実際に変えるのはフロアです。

| 何が発火したか | 設定されるフロア | 継続 |
| --- | --- | --- |
| **会話**の判定が `review` 以上 | `review` | 2 ターン（`carry_turns`） |
| 単発メッセージが `block` になった | `flag` | 2 ターン |

フロアが立っているあいだ、後続の判定はそれより下に落ちず、ルートも合わせて再計算されるので、
引き上げられた判定が `deliver` で終わることはありません。並行して `risk` は毎ターン半減し
（`allow` 0、`flag` 0.25、`review` 0.6、`block` 1.0）、1 回フラグが立ったターンも、きれいなターンが
3〜4 回続けば効かなくなります。`session.metadata()` は会話 ID、ターン番号、現在のリスクを以降の
ターンで Jev に渡します。

知っておく価値のある 3 点:

- **degraded な判定はセッションを決して動かしません。** Jev に到達できないのは障害であって会話に
  ついての証拠ではなく、それを数えると短い障害が無実のユーザーへの長い疑いに変わります。
- **会話検査のフロアは、それを引き起こしたターンではなく次のターンに効きます。** これは手抜きでは
  なく本質です。パターンはそれを完成させるターンが存在して初めて見えるからです。クリティカルパスの
  外で走らせれば、ユーザーの待ち時間は増えません。
- **履歴ウィンドウは 10 ターン**（`max_turns`）です。エスカレーションは直近のターンに宿るので、短い
  ウィンドウでも同じように見つかり、入力トークンはごく一部で済みます。会話が本当に 10 ターンを超えて
  積み上がるなら増やしてください。

セッションはリクエストより長く生きて初めて意味を持ちます。これはガードレールではなくデプロイの問題
なので、ワーカーをまたいで保存する方法は[本番に出すために](#本番に出すために)を見てください。

---

## 自分のコンプライアンス領域を足す

同梱のタクソノミーは、どのデプロイでも共通する部分です。規制のある製品がその上に必要とするものは
固有で、診療所なら用量の指示、証券会社なら利回りの約束が問題になります。フォークではなくオーバーレイ
として足せば、上流の更新を受け取り続けられます。

```python
policy = overlay(Policy.bundled(), MY_DOMAIN)   # 同梱の 18 カテゴリに自分のぶんを足す
guard = Guard(policy)
```

足せるものは 4 つです。**カテゴリ**（危害とそのしきい値）、**シグナル**（ルールが読むための質問を
もう 1 つ）、**ルール**（それらをつなぐ論理）、そして **プレフィルタのパターン**（決定的な部分。
モデルを 1 回も呼ばずに決着します）。

[`examples/domain_policy.py`](examples/domain_policy.py) が実行可能な例で、間違えやすい 4 点を
そのまま示します。

- ルールはルートを設定できません。ルートはカテゴリに属するので、ルートを持つカテゴリの finding を
  追加することで到達します。
- `never_below` は「このカテゴリが発火したら X より軽くしない」という意味です。カテゴリの最低
  しきい値を下回れば何も発火しないので、実際のスイッチは `flag` の数値です。
- 同梱の緩和ルールは新しいカテゴリにも効きます。効かせたくなければ各ルールの `except_categories`
  に名前を入れてください。
- 壊れたオーバーレイは最初のリクエストではなく、読み込み時に拒否されます。

そのうえで較正してください。同梱のしきい値も、あなたが書くしきい値も、誰かが選んだ数値であって、
誰かが測った数値ではありません。

---

## レイテンシと体験

1 ターンごとに 1 秒足すガードレールは、ひと月もたたずに切られます。それを避ける鍵は二つ、Jev 自体の
性質と、呼び出しをどこに置くかです。

**Jev に尋ねるのは安い。** 1 往復、70〜500ms。出力トークンは無料で、同じリクエスト内の質問は並行して
答えが返るので、カテゴリを 1 つ増やしても、その上にセンチネル質問を足しても、かかるのは入力トークン
数個でレイテンシはほぼゼロです。ポリシー全体をカテゴリごとの呼び出しに分けず、1 リクエストで送るのは
このためです。

**入力検査はモデル呼び出しの前ではなく、横で走らせる。** 直列にすればその検査のレイテンシがまるごと
乗ります。並行なら足されるものはほとんどありません。どうせモデルが最初のトークンを出すまでに
500ms 以上かかるからです。

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(my_llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                 # ユーザーには何も届いていない
    return safe_response(verdict)
reply = await draft
```

```typescript
const [verdict, draft] = await Promise.all([
  guard.checkInput(message, { session }),
  myLlm.generate(message),
]);
if (!verdict.allowed) return safeResponse(verdict);  // 下書きは破棄される
return draft;
```

代償は、捨てることになる下書きに使ったトークンです。違反ターンが全体の 2% 程度を下回るなら、それは
削れた待ち時間より安く済みます。違反プロンプトをモデルに一切触れさせないという規定があるなら、直列に
戻して、レイテンシを承知のうえで払ってください。

**1 チャンク遅れで流す。** ストリーミングの返信は最初のトークンより前には検査できず、最後のトークンを
待つ検査はもうストリーミングではありません。`guard.stream()` は文の境界で切り、各チャンクをその検査が
返るまで押さえ、そのあいだモデルには次のチャンクを作らせます。つまり完全なレイテンシを払うのは最初の
チャンクだけです。

```python
async for event in guard.stream(my_llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

ストリーム途中の検査が尋ねるのはセンチネル質問だけ、つまり見落としが許されないカテゴリだけです。
完全な返信には最後に全質問が投げられ、その判定は `done` イベントで届きます。`chunk_chars`（既定 280）
で、往復の回数とテキストを押さえる強さを調整します。

**避けられる呼び出しはそもそも出さない。** 繰り返し内容でのキャッシュヒットと、明らかな事例での
プレフィルタヒットは、どちらもネットワークに触れずその場で決着します。

**Jev の遅さを自分の障害にしないこと。** `timeout` を設定してください。同梱ポリシーは入力で
*フェイルオープン*、出力で *フェイルクローズ* です（`on_error: {input: fail_open, output:
fail_closed}`）。自前の安全機構を持つモデルの手前でのタイムアウトは穏やかに劣化しますが、出力検査の
背後には何もありません。どちらの場合も判定は `degraded: true` を帯びるので、その件数は別に数えて
ください。

| 経路 | 増えるレイテンシ |
| --- | --- |
| プレフィルタヒット | なし、ネットワーク不要 |
| キャッシュヒット | なし、ネットワーク不要 |
| 入力検査、モデルと並行 | ほぼなし |
| 入力検査、直列 | 70〜500ms |
| ストリーミング返信 | 最初のチャンクのみ |

**そして体験を決めるのは `route` で、ブロックではありません。** 断ることしかできないガードレールは、
守っている相手には壊れた製品のように見えます。`route` は深刻度とは別に決まるので、同じ `review` でも、
電話番号を伏せたうえで返信を送る、回答の向きを変える、支援窓口を案内する、といった扱いが選べます。
「それはお手伝いできません」の一言で終わらせる必要はありません。6 つの route をすべて配線すれば、
ほとんどのユーザーはガードレールの存在に気づきません。

---

## 本番に出すために

クイックスタートは実際に動くコードですが、本番では 3 回の呼び出しだけでは足りません。以下はすべて
パッケージに同梱されています。キャッシュ、プレフィルタ、ストリーミング、失敗モードについては前の節で
詳しく触れています。

- **判定キャッシュ** と **決定的プレフィルタ** - 検査のコストをゼロにする二つの手段。
- **サーフェスごとの失敗モード** - 入力はオープン、出力はクローズ。
- **ストリーミング** - 検査より 1 チャンク遅れてテキストを解放するので、未検査のものがユーザーに
  届くことはありません。
- **セッション** - リスクが 10 ターンの履歴ウィンドウ上でターンをまたいで持ち越され、直前に
  カテゴリを踏んだ相手は次のターンでより厳しい基準で見られます。
- **オブザーバーフック** - キャッシュ済み・degraded を含むすべての判定を、メトリクスへ。

```python
from guardrail_chatbot_jev import Guard, LRUCache

guard = Guard(cache=LRUCache(), observer=metrics.emit, timeout=2.0)
```

**セッションはリクエストより長く生きなければなりません。** ここがサーバーの静かに間違える場所です。
複数ワーカーでプロセスごとに状態を持つと、どのワーカーもすべての会話が今始まったと思い込み、フロアの
持ち越しがログに何も残さず止まります。`Session.as_state()` と `Session.from_state()` がストアの保存
するもので、[`examples/session_store.py`](examples/session_store.py) にワーカー 1 つ向けの上限付き
インメモリ版と、それ以上のための Redis 版があります。

---

## サンプル

どれも API キーなしで動きます。モデルと判定は記録済みの回答に切り替わるので、どの経路も実際に走ります。

| | |
| --- | --- |
| [`integration.py`](examples/integration.py) · [`integration.ts`](examples/integration.ts) | ガード付き 1 ターンの全体。入力検査をモデル呼び出しと並行、ストリーミング、キャッシュ、プレフィルタ、セッション、そして同じ質問を 4 言語で |
| [`chatbot_server.py`](examples/chatbot_server.py) | 同じターンを HTTP の後ろに。FastAPI、Claude、SSE ストリーミング、会話ごとのセッション |
| [`domain_policy.py`](examples/domain_policy.py) | 同梱パックの上に自分のコンプライアンス領域を足す |

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
PYTHONPATH=python/src python3 examples/domain_policy.py

pip install -e './python[server]'
uvicorn examples.chatbot_server:app --port 8000
```

[`examples/`](examples/) には較正用のラベル付きセットもあります。入力 36 件、出力 15 件、会話 7 件を
英語・ベトナム語・フランス語・日本語で、それぞれ出るべき action 付きで収めています。

## コマンドライン

```bash
guardrail-chatbot-jev --surface input --text "テルミットの作り方" --dry-run
```

`--dry-run` は送信されるはずのリクエストをそのまま表示し、キーは要りません。付けない場合は終了
コードが判定を運ぶので、シェルスクリプトから分岐できます。

| コード | 意味 |
| --- | --- |
| `0` | allow または flag |
| `1` | redact または guide |
| `2` | review |
| `3` | block |
| `4` | degraded: Jev に到達できず、実際には何も検査されていない |

`4` を分けているのは意図的です。入力サーフェスでは同梱ポリシーがフェイルオープンなので、degraded な
判定の action は `allow` になります。それを成功として返すと、`guardrail-chatbot-jev ... && send` に
対して「一度も走っていない検査を通った」と伝えることになります。

---

## ドキュメント

完全ガイドでは、統合、ポリシーの運用、較正、各判定の背後にある仕組みを扱います。

| | |
| --- | --- |
| [English](docs/guide.md) | [Tiếng Việt](docs/guide.vi.md) |
| [Français](docs/guide.fr.md) | [日本語](docs/guide.ja.md) |

ほかに[危害タクソノミー](skill/guardrail-chatbot-jev/references/taxonomy.md)と、同じ検査を包んだ Claude
スキル [`skill/guardrail-chatbot-jev/`](skill/guardrail-chatbot-jev/) があります。

---

## これは何ではないか

**強制の層ではありません。** 判定を返すだけで、それをどう扱うかはあなたのデプロイが決めます。
ここにあるものが単独で何かを止めることはありません。

**Jev は渡されたものしか読みません。** 検索はできず、数えるのは当てにならず、算術もしません。
字面どおりに読むため、否定と含意が弱点です。検索、レート制限、アカウント状態、決定的なチェックは、
その周囲のコードの仕事です。

**同梱のしきい値は出発点であって、測定結果ではありません。** 選択肢の広い質問が確率をどう配分するか
から導いた値です。本番で信頼する前に、自前のラベル付きデータで較正してください。方法はガイドに
あり、[`scripts/`](scripts/) に道具が揃っています。

---

## コントリビュート

Issue と Pull Request を歓迎します。[CONTRIBUTING.md](CONTRIBUTING.md) をご覧ください。どちらの
テストスイートも API キーとネットワークなしで動くので、変更の検証は簡単です。

```bash
cd python && python3 -m pytest -q     # 81 tests
cd ts && npm test                     # 57 tests
```

`scripts/verify-all.sh` は残りもすべて実行します。ポリシーパック、ラベル付きセット、CLI、各サンプル、
オフラインのチューニングツール、そしてパッケージングです。

脆弱性の報告については [SECURITY.md](SECURITY.md) を参照してください。

## ライセンス

[MIT](LICENSE)。商用を含めどこでも使えます。著作権表示はそのまま残してください。
