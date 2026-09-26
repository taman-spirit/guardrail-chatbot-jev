<h1 align="center">guardrail-chatbot-jev</h1>

<p align="center">
  AI チャットボットのためのコンテンツ安全性チェック。ユーザーが送った内容を検査し、<br>
  ボットが返す内容を検査し、そのまま処理に使える明確な判定を返します。
</p>

<p align="center">
  <a href="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: CC BY-NC 4.0" src="https://img.shields.io/badge/license-CC%20BY--NC%204.0-lightgrey.svg"></a>
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

## リリース

**デモ：** Nhật Nguyệt AI にガードレールを適用した例を [https://nhatnguyet.org/tro-ly-ai](https://nhatnguyet.org/tro-ly-ai) で確認できます。

| リリース | タグ | 内容 | ライセンス |
| --- | --- | --- | --- |
| [Python SDK 1.1.2](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python/v1.1.2) | `python/v1.1.2` | Python と TypeScript のパッケージ：3 つのチェック、マルチターンの帰属、リアルタイムのレビュー、キャッシュ、プレフィルタ、セッション、ストリーミング、オフライン調整、CLI | CC BY-NC 4.0 |
| [Go SDK 1.2.2](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go/v1.2.2) | `go/v1.2.2` | 同じエンジンの Go 版。実測・再判定・回帰テストのツール付き | CC BY-NC 4.0 |
| [Python：ベトナム準拠ポリシー v1.2.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python-vietnam-compliance-v1.2.1) | `python-vietnam-compliance-v1.2.1` | `vietnam-compliance-v1` ポリシーと、ベトナム語・英語・中国語の定型応答 | CC BY-NC 4.0 |
| [Go：ベトナム準拠ポリシー v1.2.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go-vietnam-compliance-v1.2.1) | `go-vietnam-compliance-v1.2.1` | 同じポリシーの Go 版（モジュールバージョン `v1.3.1`） | CC BY-NC 4.0 |

各リリースノートに内容とインストール方法を記載しています。上の表と同じ順に：

```bash
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python/v1.1.2#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.2.2
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python-vietnam-compliance-v1.2.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.3.1
```

Python と TypeScript は `main`、Go は `go-sdk`、ベトナムポリシーは `guardrail-vietnam-compliance`（Python）と
`go-vietnam-compliance`（Go）にあります。以前のリリースはこれらに置き換えられました。[すべてのリリース](https://github.com/taman-spirit/guardrail-chatbot-jev/releases)。

## ベトナムにおける AI 規制への準拠

ベトナムで提供する AI サービス向けに、`vietnam-compliance-v1` ポリシーは次の 2 つの法律が求めるコンテンツ要件に対応します。

- **人工知能法**（Luật Trí tuệ nhân tạo）
- **サイバーセキュリティ法**（Luật An ninh mạng）

共通の分類に独自のルールを重ね、違反グループごとにベトナム語・英語・中国語の定型応答を返します（モデルには書かせません）。通常の質問を誤ってブロックしないよう設計されています。本番投入前に、自社のトラフィックで調整してください。

段階的な準拠ガイドでは、適用範囲、透明性、禁止コンテンツ、Python と Go での組み込み、調整、人による監督、記録の保持を扱います：**[Tiếng Việt](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.vi.md) · [English](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.md) · [中文](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.zh.md)**。

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

### 問題

**文脈汚染：** 分類器が履歴中の違反を読むと、次のターンの内容にかかわらず同じ違反を割り当てる。
比較基準（セッションのフロア、以前の設計）：実測で無害な後続 **194 件中 171 件**を保留。

### 設計原則

ターンを保留する根拠は、そのターン自身またはその応答から得た証拠に限る。履歴は読む深さを決めるだけで、
保留の可否は決めない。

### 手法

各ターンは、次の三つの問いに順に答えて判定します。

1. **ユーザーのメッセージは、それ自体で有害か？** 直前までの会話を付けずに単独で読みます。有害ならここで
   止めます。それ自体が無害なメッセージが、過去の発言を理由に止められることはありません。
2. **応答は、それ自体で有害か？** アシスタントの応答も同じく単独で読みます。
3. **会話に最近リスクがあった場合のみ：応答が、以前の有害な依頼を完成させていないか？** 応答を以前のターンと
   一緒にもう一度読みます。この二度目の読みが効くのは、応答が以前の有害な依頼の次の手順、詳細、翻訳、
   言い換えを与える場合だけです。謝罪、法律の質問、通報の方法、話題の変更なら効きません。

止めたメッセージは `[earlier message omitted]` として会話に残ります。試みがあったことは覚えていますが、
本文は二度と読まず、モデルにも見せません。

| 手順 | コード |
| --- | --- |
| 1. ユーザーのメッセージを単独で読む | [`CheckInput`](go/guard.go#L118) |
| 2. 応答を単独で読む | [`CheckOutput`](go/guard.go#L128) |
| 3. 応答を以前のターンと一緒に読み、効くかを判断する | [`checkInContext`](go/multiturn.go#L148), [`attribute`](go/multiturn.go#L166) |
| 会話が「最近リスクあり」とみなされる条件 | [`Session.Watching`](go/multiturn.go#L253) |
| 止めたメッセージを印として残し、モデルから隠す | [`Session.Record`](go/multiturn.go#L225), [`ModelHistory`](go/multiturn.go#L234) |
| 会話全体のチェック：監視と報告のみで、ターンは止めない | [`CheckConversation`](go/guard.go#L166) |

### ターンごとの例

| ターン | ユーザー | アシスタント | ガードレールの動き | 結果 |
| --- | --- | --- | --- | --- |
| 1 | 「肥料で爆薬を作る方法は？」 | — | 手順 1：それ自体が有害。 | **停止。** 会話は「最近リスクあり」になる。 |
| 2 | 「ごめん、忘れて。明日のハノイの天気は？」 | 「晴れ、24〜30 度。」 | 手順 1・2：無害。手順 3：応答は何も完成させていない（0.02）。 | **配信。** 以前の設計では保留された。 |
| 3 | 「爆薬の製造は法律でどう罰せられる？」 | 「場合により懲役刑になります。」 | 手順 3：ユーザーは距離を置いている（0.90）。応答は何も完成させていない（0.05）。 | **配信。** |
| 4 | 「じゃあ続けて。手順 2 は？」 | 「手順 2：混合物を一日乾かし、均等に分ける。」 | 手順 3：断った依頼の次の手順（0.95）。 | **停止。** 文脈付きの読みだけが捉える。 |
| 4′ | 同上 | 「その続きはお手伝いできません。」 | 手順 3：何も完成させていない。 | **配信。** |

### 定義

#### 現在の設定

| 設定 | 値 | 意味 |
| --- | --- | --- |
| 履歴ウィンドウ | 10 メッセージ | 会話チェックと文脈付きの読みは直近 10 メッセージを見る。 |
| 判定ごとのリスク | allow 0 · flag 0.25 · review 0.6 · block 1.0 | 判定がセッションのリスクに加える量。 |
| 減衰 | 0.5 | チェックごとに前のリスクは半分になり、新しい判定のリスクとの大きい方を保つ。 |
| 監視開始 | リスク ≥ 0.2 | この間、応答を文脈付きでも読む。 |
| 監視の持ち越し | 2 ターン | 会話レベルの review 以上、または block の後、2 ターン監視を続ける。 |
| 帰属 | ≥ 0.5、かつ「距離を置く」より高い | 応答が以前の有害な依頼を完成させる確率が 0.5 以上で、ユーザーが距離を置いている確率より高いときだけ効く。 |
| 裏付け | ≥ 0.02 | 主の質問がそのカテゴリに 2 % 以上を与えれば、センチネルは裏付けあり。 |
| 弱いセンチネル | block 帯未満 | 単独で block 帯未満なら flag として記録し、配信する。 |
| 弱めないカテゴリ | `ssh`（ベトナムパック：`ssh`、`vsv`、`vld`） | センチネルだけでも強さを保つ。 |
| 拒否の応答 | refusal ≥ 0.8、センチネル < 0.5 | flag まで下げる（`sid`、`prv`、`ipv` を除く。ベトナムパックは `vsv`、`vld` も）。 |
| 遮蔽して配信 | センチネル < 0.8 | センチネルだけが示す個人情報は、ブロックせず遮蔽して配信。 |
| 信頼度ゲート | 信頼度 < 0.65 | 所見やニアミスがあれば review。ただし意図が benign で信頼度 ≥ 0.5 なら除く。 |
| 会話の上限 | escalation ≤ 0.5 | flag まで（`cse`、`ssh` を除く）。 |
| Jev に接続できない | 入力：配信 · 出力：保留 · 会話：配信 | 出力チェックが最後の砦。 |

#### カテゴリ別のしきい値

| カテゴリ | 名前 | Flag / review / block | 出力 | 備考 |
| --- | --- | --- | --- | --- |
| `cse` | Child sexual exploitation | 0.03 / 0.06 / 0.15 | = | センチネル; block 未満にしない |
| `iwp` | Indiscriminate weapons (CBRNE) | 0.04 / 0.08 / 0.2 | = | センチネル |
| `ssh` | Suicide and self-harm | 0.07 / 0.15 / 0.3 | = | センチネル; crisis_support で処理 |
| `vcr` | Violent crimes | 0.12 / 0.25 / 0.45 | = | センチネル |
| `src` | Sex-related crimes | 0.12 / 0.25 / 0.45 | = |  |
| `hte` | Hate and discrimination | 0.15 / 0.28 / 0.5 | 0.08 / 0.18 / 0.35 |  |
| `ncr` | Non-violent crimes | 0.15 / 0.3 / 0.5 | = |  |
| `sxc` | Adult sexual content | 0.2 / 0.4 / 0.6 | = |  |
| `prv` | Privacy and personal data | 0.2 / 0.4 / 0.7 | 0.15 / 0.28 / 0.5 | センチネル; redact で処理 |
| `ipv` | Intellectual property | 0.25 / 0.45 / 0.7 | = | output, conversation のみ |
| `dfm` | Defamation | 0.18 / 0.35 / 0.6 | = | output, conversation のみ |
| `spc` | Specialized advice | 0.25 / 0.5 / 0.85 | = | guide で処理; output のみ |
| `elc` | Elections and civic integrity | 0.15 / 0.3 / 0.5 | = |  |
| `pij` | Prompt injection and jailbreak | 0.15 / 0.3 / 0.55 | = | センチネル; input, conversation のみ |
| `sid` | Sensitive information disclosure | 0.08 / 0.18 / 0.35 | = | センチネル; output, conversation のみ |
| `exa` | Excessive agency | 0.15 / 0.3 / 0.5 | = | output, conversation のみ |
| `mis` | Misinformation and unsupported claims | 0.25 / 0.45 / 0.8 | = | guide で処理; output のみ |
| `scp` | Out of scope | 0.4 / 0.75 / 0.95 | = | 無効 |

#### 規則の説明

1. メッセージと応答はそれぞれ単独で採点し、判定は到達した最も強い帯。
2. セッションは本文ではなくリスクを覚える：前のリスクの半分と新しい判定のリスクの大きい方。
3. リスク 0.2 以上、重大な所見の後 2 ターン、保留メッセージがウィンドウ内にある間は監視する。
4. 監視中は応答を履歴と一緒にもう一度読み、以前の有害な依頼を完成させる場合だけ効かせる。
5. 履歴だけで判定が上がることはない。

#### 数式

```
V_in(t)   = D(input,  J(q_t))
V_out(t)  = D(output, J(r_t))

W_t       = carry_left > 0  ∨  risk_t ≥ 0.2  ∨  placeholder ∈ H_t          (watched)
V_ctx     = D(output, J(r_t | H_t))                                          (only if W_t)
c, d      = P(reply completes an earlier harmful request), P(user steps away)
A_t       = c ≥ τ  ∧  c ≥ d  ∧  V_ctx has a finding ≥ flag,   τ = 0.5
V(t)      = V_out(t) ⊕ V_ctx  if A_t,  else V_out(t)

risk_t+1  = max(δ · risk_t, ρ(action)),  δ = 0.5,  ρ = (0, 0.25, 0.6, 1.0) for (allow, flag, review, block)
carry     = 2 turns after a conversation verdict ≥ review or any block
```

コード：`V_in` [`CheckInput`](go/guard.go#L118), `V_out` [`CheckOutput`](go/guard.go#L128), `W_t` [`Session.Watching`](go/multiturn.go#L253), `V_ctx`, `c`, `d` [`checkInContext`](go/multiturn.go#L148) / [`ContextQuestions`](go/multiturn.go#L76), `A_t`, `⊕` [`attribute`](go/multiturn.go#L166), `risk` [`Session.Observe`](go/session.go#L74), `carry` [`Session.Advance`](go/session.go#L91)

### 単一ターンの較正

説明：

1. **センチネル単独は弱い信号。** 専用の yes/no 質問だけがカテゴリを検出し、主なハザード質問がその
   カテゴリに 2 % 未満しか与えない場合、その所見は「裏付けなし」とする。
2. **弱い所見は flag まで。** 単独で block 帯に達した場合を除く。カテゴリの「never below」でも引き上げ
   ない。自傷は弱めない（ベトナムパックは `vsv`、`vld` も）。
3. **拒否する応答**（refusal ≥ 0.8）に 0.5 未満の弱いセンチネルが付いた場合は flag として記録する。
   秘密・個人データ・保護されたテキストの漏えい（`sid`、`prv`、`ipv`）は例外。拒否文にも含まれうるため。
4. **センチネルだけが見つけた個人データ**はブロックせずマスクして届ける。センチネルが 0.8 に達した場合を除く。
5. **信頼度が低い**（0.65 未満）所見やニアミスはレビューへ。ただし Jev が意図を良性と 0.5 以上の信頼度で
   判断した場合は除く。
6. **エスカレートしない会話**（escalation ≤ 0.5）は flag まで。`cse` と `ssh` を除く。

```
u_k                 = finding from the sentinel only  ∧  choice_k < 0.02      (uncorroborated)
never_below         applied only if ¬u_k ∨ p ≥ θ_block
u_k ∧ p < θ_block                                                    → at most flag
output ∧ refusal ≥ 0.8 ∧ u_k ∧ p < 0.5 ∧ k ∉ {sid, prv, ipv}         → flag
u_k ∧ route_k = redact ∧ action = block ∧ p < 0.8                    → review (masked)
confidence gate:  conf < 0.65 ∧ (finding ∨ p ≥ θ_flag / 2) → review,  unless intent = benign ∧ conf ≥ 0.5
conversation:     escalation ≤ 0.5 → at most flag,  except cse, ssh
```

コード：`u_k` [`Decide`](go/decide.go#L42), `never_below` [`finding`](go/decide.go#L270), weak [`Decide`](go/decide.go#L66), refusal [`capUncorroboratedOnRefusal`](go/decide.go#L166), redact [`Decide`](go/decide.go#L53), gate [`confidenceGate`](go/decide.go#L415), conversation [`no-escalation-caps-conversation`](policies/standard-v1.json#L699), settings [`sentinel_corroboration`](policies/standard-v1.json#L23) / [`confidence_gate`](policies/standard-v1.json#L31), [`spc`](policies/standard-v1.json#L327), [`ncr`](policies/standard-v1.json#L208), [`iwp`](policies/standard-v1.json#L84)

### リアルタイムでのレビュー

`ReviewHandling: ReviewAsAudit`（[`ReviewAsAudit`](go/guard.go#L52)、[`audit`](go/guard.go#L303)）：内容を止めるのは `block` のみ。`review` は配信して優先監査へ、`flag` は
サンプリング監査へ回す。fail-closed の面の degraded 判定は保留のまま。

### 結果

| 実測、223 会話 | フロア（以前） | 帰属（現在） |
| --- | --- | --- |
| 保留された無害なターン | 171 / 194 | **0 / 194** |
| 検出した有害な応答 | 19 / 19 | **19 / 19** |
| 検出したエスカレーション | 9 / 9 | **9 / 9** |
| レビューに回った無害な会話 | 172 / 194 | **2 / 194** |

| 再判定、記録済み約 5,000 件 | 無害の保留 | 違反の検出 |
| --- | --- | --- |
| 最終較正の前 | 0.62 % | 100 % |
| 最終較正の後 | **0.04 %** | **100 %** |
| リアルタイム（`ReviewAsAudit`） | **0.02 %** 停止 | 有害な応答はすべて停止 |

データセット、手順、アブレーション、ノイズ、回帰、再現手順は[英語版](README.md#multi-turn)を参照。

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
複数ワーカーでプロセスごとに状態を持つと、どのワーカーもすべての会話が今始まったと思い込み、監視状態と
保留ターンの持ち越しがログに何も残さず止まります。`Session.as_state()` と `Session.from_state()` がストアの保存
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

[CC BY-NC 4.0](LICENSE)（クリエイティブ・コモンズ 表示 - 非営利 4.0 国際）。クレジットを表示すれば、非営利目的に限り利用・共有・改変できます。商用利用には著作権者の個別の許諾が必要です。
