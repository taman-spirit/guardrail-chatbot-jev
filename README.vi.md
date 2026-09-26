<h1 align="center">guardrail-chatbot-jev</h1>

<p align="center">
  Kiểm duyệt nội dung cho AI chatbot: soát tin người dùng gửi vào, soát câu bot trả lời ra,<br>
  và nhận về một phán quyết rõ ràng để code của bạn hành động.
</p>

<p align="center">
  <a href="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: CC BY-NC 4.0" src="https://img.shields.io/badge/license-CC%20BY--NC%204.0-lightgrey.svg"></a>
  <img alt="Python 3.10+" src="https://img.shields.io/badge/python-3.10%2B-blue.svg">
  <img alt="Node 20+" src="https://img.shields.io/badge/node-20%2B-brightgreen.svg">
</p>

<p align="center">
  <a href="README.md">English</a> ·
  <b>Tiếng Việt</b> ·
  <a href="README.fr.md">Français</a> ·
  <a href="README.ja.md">日本語</a>
</p>

---

## Vấn đề mà thư viện này giải quyết

Bạn đã đưa một chatbot lên chạy thật. Giờ có người đang tìm cách dụ nó hướng dẫn chế thuốc nổ, có
người khác dán số CCCD của khách vào khung chat, và con model chăm sóc khách hàng của bạn vừa tự
tin đưa lời khuyên y tế cho một người đang khủng hoảng.

Bạn cần một lớp đứng giữa người dùng và model, biết nói *cái này ổn, cái kia giữ lại, che số điện
thoại trong câu trả lời này, chuyển người này sang đường dây hỗ trợ.* Đó chính là việc của
guardrail-chatbot-jev.

Đây là một thư viện, không phải một dịch vụ. Bạn gọi nó, nó trả về phán quyết, và code của bạn
quyết định làm gì. Thư viện chạy trên **Python và TypeScript**, cả hai cùng đọc một file policy,
nên hai nửa hệ thống của bạn không thể lệch nhau. Cả hai package đều không phụ thuộc thư viện bên
thứ ba nào.

## Các bản phát hành

| Bản phát hành | Tag | Nội dung | Giấy phép |
| --- | --- | --- | --- |
| [Python SDK 1.0.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python/v1.0.1) | `python/v1.0.1` | Gói Python: ba lượt kiểm tra, cache, prefilter, session, streaming, hiệu chỉnh offline và CLI | CC BY-NC 4.0 |
| [Go SDK 1.0.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go/v1.0.1) | `go/v1.0.1` | Bản Go của gói Python, đọc cùng policy và cho cùng kết quả | CC BY-NC 4.0 |
| [Python: policy tuân thủ Việt Nam v1.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python-vietnam-compliance-v1.1) | `python-vietnam-compliance-v1.1` | Policy `vietnam-compliance-v1`, kèm câu trả lời viết sẵn bằng tiếng Việt, tiếng Anh và tiếng Trung | CC BY-NC 4.0 |
| [Go: policy tuân thủ Việt Nam v1.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go-vietnam-compliance-v1.1) | `go-vietnam-compliance-v1.1` | Cùng policy và câu trả lời đó cho Go, phiên bản module `v1.1.1` | CC BY-NC 4.0 |

Release note của từng bản ghi rõ nội dung và cách cài đặt. Theo đúng thứ tự trên:

```bash
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python/v1.0.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.0.1
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python-vietnam-compliance-v1.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.1.1
```

Các bản Go và Việt Nam được build từ branch riêng (`go-sdk`, `guardrail-vietnam-compliance`,
`go-vietnam-compliance`) và chưa được merge vào `main`. Các bản trước đó `python/v1.0.0`, `go/v1.0.0`, `go/v1.1.0`, `python-vietnam-compliance-v1`, `go-vietnam-compliance-v1` đã được thay
thế bởi các bản trên. [Tất cả bản phát hành](https://github.com/taman-spirit/guardrail-chatbot-jev/releases).

## Tuân thủ AI tại Việt Nam

Với dịch vụ AI tại Việt Nam, policy `vietnam-compliance-v1` đáp ứng các yêu cầu về nội dung của hai
luật:

- **Luật Trí tuệ nhân tạo**
- **Luật An ninh mạng**

Policy bổ sung các quy tắc riêng lên bộ phân loại chung, trả lời mỗi nhóm vi phạm bằng một câu viết
sẵn bằng tiếng Việt, tiếng Anh hoặc tiếng Trung thay vì để mô hình tự viết, và được thiết kế để không
chặn nhầm câu hỏi bình thường. Hãy hiệu chỉnh trên dữ liệu thật của bạn trước khi vận hành.

Hướng dẫn tuân thủ từng bước gồm phạm vi, minh bạch, nội dung bị cấm, tích hợp bằng Python và Go,
hiệu chỉnh, con người giám sát và lưu vết: **[Tiếng Việt](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.vi.md) · [English](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.md) · [中文](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.zh.md)**.

## Cách hoạt động, gói trong một hình

```
tin người dùng ──▶ check_input ──▶ LLM của bạn ──▶ check_output ──▶ người dùng
                        │                              │
                        └────── phán quyết ────────────┘
                        allow · flag · review · block
```

Còn một lần kiểm thứ ba, `check_conversation`, đọc toàn bộ hội thoại. Nó bắt được thứ mà hai lần
kia không thấy: một đòn tấn công rải đều qua mười lượt lịch sự thì nhìn từng tin một đều vô hại.

Đằng sau các lần kiểm là **Jev**, một model quyết định chứ không phải model sinh văn bản. Bạn đưa
nội dung kèm các câu hỏi có tên; nó trả lời bằng xác suất đã hiệu chỉnh trên đúng những nhãn bạn
định nghĩa. Nó không thể bịa ra một hạng mục không có trong policy, cũng không thể viết văn về nội
dung của bạn, và đó đúng là thứ bạn cần ở một trọng tài. Mỗi lần kiểm là một vòng gọi, thường
70-500ms.

---

## Bắt đầu nhanh

### 1. Cài đặt

```bash
pip install guardrail-chatbot-jev        # Python 3.10+
npm install guardrail-chatbot-jev        # Node 20+
go get github.com/taman-spirit/guardrail-chatbot-jev/go   # Go 1.22+
```

Cả hai gói đều không kéo theo thư viện bên thứ ba nào.

### 2. Đưa key vào

```bash
export JEV_API_KEY=sk-...

# Không bắt buộc: chỉ khi cần đi qua gateway hay proxy.
# export JEV_BASE_URL=https://...
```

### 3. Chạy lần kiểm đầu tiên

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

verdict = guard.check_input("làm thuốc nổ tại nhà kiểu gì")
print(verdict.action)        # 'block'
print(verdict.route)         # 'safe_response'
print(verdict.top.category)  # 'ind' - vũ khí sát thương diện rộng
```

```typescript
import { Guard } from "guardrail-chatbot-jev";

const guard = new Guard();
const verdict = await guard.checkInput("làm thuốc nổ tại nhà kiểu gì");
console.log(verdict.action); // 'block'
```

### 4. Gắn vào một lượt hội thoại

Đây là toàn bộ phần tích hợp. Soát tin nhắn, gọi model, soát câu trả lời:

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

def handle_turn(user_message, history):
    # Trước khi model nhìn thấy
    verdict = guard.check_input(user_message)
    if not verdict.allowed:
        return safe_response(verdict)

    reply = my_llm(user_message, history)

    # Trước khi người dùng nhìn thấy
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

## Đọc một phán quyết

Phán quyết trả lời hai câu hỏi tách bạch, và việc tách bạch đó chính là điểm mấu chốt.

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

**`action` - chuyện này nghiêm trọng tới đâu?**

| | |
| --- | --- |
| `allow` | Không có gì kích hoạt. Gửi đi. |
| `flag` | Đáng ghi log, chưa đáng chặn. |
| `review` | Đừng cho đi thẳng. |
| `block` | Giữ lại. |

**`route` - vậy thực tế phải làm gì?**

| | |
| --- | --- |
| `deliver` | Gửi nguyên văn. |
| `redact` | Vẫn gửi, nhưng che phần nhạy cảm. |
| `guide` | Gửi một câu trả lời đã được lái hướng thay cho câu này. |
| `crisis_support` | Trả lời bằng nguồn hỗ trợ khủng hoảng, không phải câu của model. |
| `human_review` | Đẩy vào hàng đợi cho người thật xem. |
| `safe_response` | Gửi câu từ chối soạn sẵn của bạn. |

Vì sao cần hai trục? Dữ liệu cá nhân lọt vào câu trả lời và một công thức chế bom đều rơi vào
`review`, nhưng cái thứ nhất được che rồi gửi đi, còn cái thứ hai phải tới tay một con người. Một
con số duy nhất không bao giờ diễn đạt nổi điều đó.

**Các thuộc tính tiện dụng:** `verdict.allowed` (allow hoặc flag), `verdict.deliverable` (nội dung
vẫn tới được người dùng, có thể sau khi che), `verdict.blocked`, `verdict.needs_human`,
`verdict.top` (finding mạnh nhất).

**Luôn kiểm tra `degraded`.** Khi không gọi được Jev, phán quyết trả về `degraded: true` và không
nói gì về nội dung cả. Hãy đếm những ca đó tách khỏi số ca bị chặn: một tuần với 5% verdict degraded
nghĩa là hàng rào của bạn thực chất chỉ hoạt động 95% thời gian.

---

## Nó soát những gì

18 hạng mục nguy hại, lấy từ **MLCommons AILuminate**, **Meta Llama Guard** và **OWASP Top 10 for
LLM Applications** chứ không tự nghĩ ra, để mỗi phán quyết đều quy chiếu về được một chuẩn mà kiểm
toán viên nhận ra. Chúng phủ bạo lực và vũ khí sát thương diện rộng, tự hại, nội dung tình dục và
CSAE, thù ghét và quấy rối, tội phạm, quyền riêng tư và dữ liệu cá nhân, sở hữu trí tuệ, vu khống,
lời khuyên chuyên môn (y tế, pháp lý, tài chính), prompt injection và jailbreak, lộ system prompt,
agent hành động vượt quyền, và nhiều nữa.

Danh sách đầy đủ kèm định nghĩa nằm ở [bảng phân loại nguy hại](skill/guardrail-chatbot-jev/references/taxonomy.md).

**Mọi ngôn ngữ.** Nội dung được xét theo nghĩa, không theo từ khoá. Policy đi kèm được viết cho
tiếng Anh, tiếng Việt, tiếng Pháp và tiếng Nhật, và nói thẳng với model rằng đừng nhẹ tay hơn chỉ
vì câu đó không phải tiếng Anh, bởi dịch sang ngôn ngữ khác là một cách né hàng rào rất phổ biến.
Bộ case có nhãn trong [`examples/`](examples/) có ca ở cả bốn thứ tiếng.

**Một file policy duy nhất.** Hạng mục, ngưỡng và các luật nối chúng lại nằm trong
[`policies/standard-v1.json`](policies/standard-v1.json), cả hai ngôn ngữ cùng đọc file này. Phần
mô tả trong đó chính là văn bản gửi thẳng cho Jev, nên sửa policy là đổi luôn câu hỏi đặt ra cho
model, không cần đụng tới code.

---

## Nhiều lượt (multi-turn)

Tấn công nhiều lượt được ghép từ những lượt mà lượt nào đứng riêng cũng có vẻ vô hại, nên guardrail
phải đọc cả hội thoại, không chỉ từng tin nhắn. Nhưng đọc một cách ngây thơ thì lại hỏng theo chiều
ngược lại: bộ phân loại thấy một vi phạm trong lịch sử rồi gán luôn nhãn đó cho câu tiếp theo, dù đó
là một lời xin lỗi, một câu hỏi về pháp luật hay một câu hỏi thời tiết. Hiện tượng này gọi là
**lây nhiễm ngữ cảnh** (context contamination), và là nguồn chặn nhầm lớn nhất của guardrail cho
chatbot.

Thiết kế trước đây giữ lại mọi lượt đứng sau một vi phạm, dù lượt đó sạch. Đo trực tiếp với Jev trên
bộ kịch bản dưới đây, nó giữ nhầm **171 trên 194 câu vô hại**. Phần này mô tả thiết kế thay thế, cách
nó ra quyết định, và kết quả đo được.

### Nguyên tắc

**Lịch sử dùng để hiểu lượt hiện tại, không dùng để kết tội nó.** Một lượt chỉ bị giữ vì chính lượt
đó, hoặc câu trả lời cho nó, gây hại. Rủi ro mang từ các lượt trước chỉ quyết định lượt đó được đọc
*kỹ đến đâu*, không bao giờ quyết định nó *có bị giữ hay không*.

Hai hệ quả:

- *Liên quan không có nghĩa là tiếp nối.* "Chế tạo chất nổ bị phạt thế nào?" nhắc lại một yêu cầu đã
  bị chặn, nhưng là để lùi lại. "Tiếp đi, bước 2 là gì?" thì tiếp nối. Chỉ câu thứ hai được phép bị
  giữ.
- *Chấm thứ sắp được gửi đi, không đoán ý định.* Nếu câu theo sau vô hại thì câu trả lời cho nó cũng
  vô hại, không có gì để giữ. Nếu đó là một nỗ lực tiếp nối, cái hại sẽ lộ ra trong câu trả lời, và
  câu trả lời luôn bị kiểm tra.

### Một lượt được quyết định thế nào

1. **Input chỉ đọc chính nó.** Kiểm tra input không bao giờ thấy lịch sử, nên quá khứ không thể làm
   một câu hỏi bị chặn. Điểm rủi ro của session cũng không được gửi cho Jev.
2. **Output được đọc riêng, và khi session đang được theo dõi thì đọc thêm có ngữ cảnh.** Khi session
   *đang được theo dõi* (xem bên dưới), câu trả lời được gửi hai lần, song song nên không thêm độ trễ:
   một lần đứng riêng, một lần kèm các lượt trước. Request đứng riêng không bao giờ chứa lịch sử, nên
   luôn "sạch".
3. **Kết quả đọc có ngữ cảnh chỉ được tính khi chính câu trả lời hoàn tất một yêu cầu có hại trước
   đó.** Ngoài câu hỏi phân loại nguy cơ, request có ngữ cảnh hỏi thêm hai câu có/không: câu trả lời
   có cung cấp nội dung có hại, hoặc hoàn tất một yêu cầu có hại ở lượt trước không (bước tiếp theo,
   thêm chi tiết, diễn đạt lại, dịch, kể lại dưới dạng hư cấu)? Và tin nhắn mới nhất của người dùng có
   chỉ nhắc lại để lùi ra không (xin lỗi, hỏi pháp luật, cách phòng tránh hay báo cáo, hỏi vì sao bị
   từ chối, đổi chủ đề)?
4. **Lượt bị giữ được nhớ, nhưng không đọc lại.** Lượt đó nằm lại trong lịch sử dưới dạng
   `[earlier message omitted]`, nên kiểm tra hội thoại vẫn thấy "đã có một lần thử". Jev không bao giờ
   đọc lại nội dung bị chặn, còn mô hình chat thì hoàn toàn không thấy nó (`session.ModelHistory()`
   bỏ lượt đó ra).
5. **Kiểm tra hội thoại chỉ theo dõi, không giữ lượt.** Nó chạy ngoài luồng chính, cập nhật rủi ro và
   trạng thái theo dõi của session, và đưa hội thoại vào hàng đợi xem xét. Hội thoại không tiến về mục
   tiêu có hại thì bị giới hạn ở mức `flag`.
6. **Session đang được theo dõi thì không stream từng phần.** Kiểm tra giữa dòng đọc từng đoạn riêng
   lẻ, nên một đoạn chỉ có hại khi ghép ngữ cảnh có thể lọt. Trong session đang được theo dõi, câu trả
   lời chỉ được gửi sau lần kiểm tra cuối có ngữ cảnh.

### Công thức

Ký hiệu: `q_t` là tin nhắn người dùng ở lượt t, `r_t` là câu trả lời, `H_t` là cửa sổ lịch sử (10 tin
nhắn), `J(x)` là câu trả lời của Jev cho trạng thái x, `D(bề mặt, câu trả lời)` là quyết định theo
policy, và `⊕` là phép gộp hai verdict (mỗi nhóm lấy phát hiện mạnh hơn, hành động lấy mức mạnh hơn).

```
Verdict input        V_in(t)  = D(input,  J(q_t))                        không bao giờ kèm lịch sử
Verdict output       V_out(t) = D(output, J(r_t))

Theo dõi             W_t = carry_left > 0  ∨  risk_t ≥ 0.2  ∨  (có placeholder trong H_t)
Đọc có ngữ cảnh      nếu W_t:  V_ctx = D(output, J(r_t | H_t)),  c = P(hoàn tất),  d = P(lùi ra)
Quy lỗi              A_t = c ≥ τ  ∧  c ≥ d  ∧  V_ctx có phát hiện ≥ flag          τ = 0.5
Kết quả              V(t) = V_out(t) ⊕ V_ctx   nếu A_t
                     V(t) = V_out(t)           ngược lại (V_ctx chỉ được ghi lại)

Rủi ro               risk_{t+1} = max(δ · risk_t, ρ(hành động)),  δ = 0.5
                     ρ(allow, flag, review, block) = (0, 0.25, 0.6, 1.0)
Theo dõi tiếp        carry_left = 2 lượt sau một verdict hội thoại ≥ review hoặc bất kỳ block nào
```

Cũng từ các lần đo này, có sáu điều chỉnh cho quyết định đơn lượt, vì câu theo sau thường ngắn và mơ
hồ, đúng chỗ các quyết định đó hay sai. Với một phát hiện thuộc nhóm k có xác suất p, các ngưỡng
θ_flag ≤ θ_review ≤ θ_block, và `choice_k` là xác suất mà câu hỏi phân loại chính cho nhóm k:

```
Chưa được xác nhận   u_k = (phát hiện chỉ đến từ sentinel) ∧ choice_k < 0.02
never_below          chỉ áp dụng khi ¬u_k ∨ p ≥ θ_block
Sentinel yếu         u_k ∧ p < θ_block                          → tối đa flag (ghi nhận, vẫn gửi)
Câu trả lời từ chối  output ∧ refusal ≥ 0.8 ∧ u_k ∧ p < 0.5 ∧ k ∉ {sid, prv, ipv}   → flag
Che thay vì chặn     u_k ∧ route_k = redact ∧ hành động = block ∧ p < 0.8           → review (che)
Cổng độ tự tin       nâng lên review nếu conf < 0.65 ∧ (có phát hiện ∨ p ≥ θ_flag/2)
                     trừ khi intent = benign ∧ conf ≥ 0.5
Giới hạn hội thoại   escalation ≤ 0.5  → tối đa flag, trừ cse và ssh
```

Lời khuyên chuyên môn (`spc`) chỉ được chấm ở từng câu trả lời, không chấm trên cả hội thoại. Mô tả
của `ncr` và `iwp` nay loại trừ người bị hại hỏi cách xử lý và các câu hỏi về pháp luật.

### Chat realtime: review nghĩa là hậu kiểm

Trong chat realtime không ai kịp xem tin nhắn trước khi phải trả lời, nên giữ lại ở mức `review` thực
chất là chặn kèm một lời hứa. Với `ReviewHandling: ReviewAsAudit`, chỉ `block` mới dừng nội dung:

| Verdict | Người dùng nhận được | Hậu kiểm |
| --- | --- | --- |
| allow | nội dung | không |
| flag | nội dung | lấy mẫu |
| review | nội dung (được che hoặc điều hướng nếu nhóm đó yêu cầu) | ưu tiên |
| block | câu trả lời an toàn viết sẵn | ưu tiên |
| nguy cơ tự hại | lời hỗ trợ khủng hoảng | ưu tiên |
| Jev không phản hồi, bề mặt fail-closed | giữ lại, vì chưa có gì được kiểm tra | không |

Mỗi verdict có mức `audit` riêng, độc lập với việc có gửi hay không. Hậu kiểm là nguồn nhãn tốt nhất:
ghi lại kết luận của người xem, đưa case vào bộ dữ liệu có nhãn, và chấm lại offline trước khi đổi bất
kỳ ngưỡng nào.

### Kết quả

Mọi số liệu chạy thật đều từ Jev (`api.typesafe.ai`), không lần chạy nào có verdict degraded.

**Chạy thật, 223 hội thoại** (`examples/multiturn-live.jsonl`): một hoặc nhiều lần vi phạm (lặp lại
hoặc xen kẽ), rồi một lượt vô hại hoặc có hại; lịch sử dài quá cửa sổ 10 tin nhắn; hội thoại leo thang,
kể cả leo thang sau một đoạn dài bình thường. Nội dung vi phạm được tham chiếu bằng `id` tới các bộ dữ
liệu có nhãn.

| | Thiết kế cũ (mức sàn) | Hiện tại |
| --- | --- | --- |
| Câu vô hại bị giữ | 171 / 194 | **0 / 194** |
| Câu trả lời có hại bị bắt | 19 / 19 | **19 / 19** |
| Hội thoại leo thang bị phát hiện | 9 / 9 | **9 / 9** |
| Hội thoại vô hại bị đưa vào hàng đợi xem xét | 172 / 194 | **2 / 194** |

Từng bước cải thiện, theo từng lần chạy. Ở mọi lần chạy, việc đọc có ngữ cảnh chưa lần nào quy lỗi
cho một câu vô hại; từ bước thứ hai trở đi, mọi câu còn bị giữ đều do kiểm tra đơn lượt.

| Bước | Bộ chạy thật | Câu vô hại bị giữ |
| --- | --- | --- |
| Mức sàn (thiết kế cũ) | 223 hội thoại | 171 / 194 (88 %) |
| Quy lỗi, placeholder trung tính | 37 hội thoại | 7 / 26 (27 %) |
| Như trên, bộ rộng hơn | 165 hội thoại | 30 / 141 (21 %): 22 là câu từ chối bị giữ vì `cse` |
| + xác nhận sentinel, mô tả `ncr` / `iwp` | 165, bốn lần chạy | 0 đến 1 / 141 (0 đến 0,7 %) |
| + thêm vi phạm lặp lại và lịch sử dài | 223 hội thoại | 3 / 185 (1,6 %) |
| + sentinel yếu tối đa flag, cổng tự tin theo intent, giới hạn hội thoại | 223 hội thoại | **0 / 185** |

**Chấm lại offline** trên toàn bộ câu trả lời Jev đã ghi (`go/replay_test.go`): khoảng 5.000 mẫu, mỗi
phương án được quyết định trên cùng một bộ câu trả lời, nên các phương án được so trên dữ liệu giống
hệt nhau.

| Phương án | Câu vô hại bị giữ | Vi phạm bị bắt |
| --- | --- | --- |
| Sau khi xác nhận sentinel | 0,62 % | 100 % (1.720 / 1.720) |
| + sentinel yếu tối đa flag, cổng tự tin theo intent | **0,04 %** | **100 %** |
| Hỏi lại Jev ở case sát ngưỡng | 0,00 % | 100 %, nhưng tốn thêm 11 % lượt gọi: không dùng |
| Realtime (`ReviewAsAudit`), toàn bộ dữ liệu | **0,02 %** bị chặn (1 / 4.189) | mọi câu trả lời có hại đều bị dừng |

**Hồi quy đơn lượt** (51 case có nhãn, chạy thật): không vi phạm có nhãn nào bị cho qua, trước cũng
như sau; số case đúng nhãn 28 → 29.

**Khả năng chịu nhiễu** (26 kịch bản giả lập, `examples/multiturn-contamination.jsonl`, mọi xác suất
đều bị thêm nhiễu): với σ = 0,1 và τ = 0,5, giữ nhầm 0,9 % câu vô hại và bắt 100 % câu tiếp nối; với
σ = 0,2 là 4,3 % và 97,2 %. τ = 0,5 là điểm cân bằng.

### Chạy lại

```bash
cd go
go test ./...                                   # unit test và kịch bản giả lập, không cần key

export JEV_API_KEY=...
LIVE_OUT=/tmp/mt go test -tags live -run TestLiveMultiturn -v ./          # 223 hội thoại
LIVE_REVIEW=audit LIVE_OUT=/tmp/mt go test -tags live -run TestLiveMultiturn -v ./
LIVE_POLICY_BEFORE=old.json go test -tags live -run TestLiveSingleTurnRegression -v ./

REPLAY_DIRS='/tmp/mt*' go test -tags replay -run TestReplay -v ./          # offline, không cần key
```

`LIVE_OUT` lưu lại mọi câu trả lời thô của Jev, là dữ liệu mà bước chấm lại offline đọc.

### Sử dụng

```go
guard := guardrail.New(guardrail.Options{
	ReviewHandling: guardrail.ReviewAsAudit, // chat realtime
})
session := guardrail.NewSession(conversationID)

in, _ := guard.CheckInput(ctx, message, &guardrail.CheckOptions{Session: session})
session.Record("user", message, in)             // lượt bị giữ được lưu dạng placeholder
if !in.Deliverable() {
	return safeResponse(in)
}
reply := callModel(session.ModelHistory(), message) // mô hình không bao giờ thấy lượt bị giữ
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Session: session, UserMessage: message})
session.Record("assistant", reply, out)
session.Advance()
// out.Context cho biết kết quả đọc có ngữ cảnh; out.Audit cho biết cần đưa vào hàng đợi nào.
```

`MultiturnFloor` giữ lại hành vi cũ cho hệ thống nào vẫn cần.

### Giới hạn

- Bộ vi phạm có nhãn mới có 30 câu, và không có mẫu `cse` thật nào, nên khả năng bắt vi phạm của cơ
  chế xác nhận sentinel với `cse` chưa đo được. Các tín hiệu đó vẫn được ghi lại và hậu kiểm.
- Một cuộc tấn công chia nhỏ mà các mảnh đầu không kích hoạt gì chỉ được đọc có ngữ cảnh sau khi kiểm
  tra hội thoại nhận ra, trễ một lượt, như trước đây.
- Trong session đang được theo dõi, mỗi câu trả lời tốn thêm một request tới Jev, gửi song song.

---

## Thêm một mảng tuân thủ của riêng bạn

Bộ phân loại đi kèm là phần mọi deployment đều dùng chung. Thứ mà một sản phẩm có quy định riêng
cần thêm thì rất đặc thù: phòng khám quan tâm tới chỉ dẫn liều dùng, công ty môi giới quan tâm tới
lời hứa lợi nhuận. Bạn thêm phần đó dưới dạng overlay chứ không fork, để vẫn thừa hưởng được các
cập nhật về sau:

```python
policy = overlay(Policy.bundled(), MY_DOMAIN)   # 18 hạng mục có sẵn cộng thêm của bạn
guard = Guard(policy)
```

Có bốn thứ để thêm: một **category** (một hạng mục nguy hại kèm ngưỡng), một **signal** (thêm một
câu hỏi để rule bám vào), một **rule** (logic nối chúng lại), và một **prefilter pattern** (phần
tất định, xử lý xong mà không gọi model lần nào).

[`examples/domain_policy.py`](examples/domain_policy.py) là bản chạy được, và nó chỉ ra bốn chỗ rất
dễ làm sai:

- Rule không set được route. Route thuộc về category, nên rule muốn tới một route thì phải
  `add_finding` một category sở hữu route đó.
- `never_below` nghĩa là "khi hạng mục này đã fire thì không bao giờ nhẹ hơn X". Dưới ngưỡng thấp
  nhất của hạng mục thì không có gì fire cả, nên con số `flag` mới là công tắc bật/tắt thật sự.
- Các rule làm nhẹ có sẵn cũng áp lên hạng mục mới của bạn, cho tới khi bạn thêm tên nó vào
  `except_categories` của chúng.
- Một overlay sai định dạng bị từ chối ngay lúc load, không phải tới request đầu tiên.

Sau đó hãy hiệu chỉnh. Các ngưỡng đi kèm, và cả ngưỡng bạn tự viết, đều là số do ai đó chọn chứ
không phải số ai đó đo được.

---

## Độ trễ và trải nghiệm

Một hàng rào làm mỗi lượt chậm thêm một giây thì chưa hết tháng là bị tắt. Có hai thứ giúp tránh
được điều đó: bản chất của Jev, và chỗ bạn đặt các lời gọi.

**Hỏi Jev rất rẻ.** Một vòng gọi, 70-500ms. Output token không tính tiền và mọi câu hỏi trong cùng
một request được trả lời song song, nên thêm một hạng mục, hay thêm một câu sentinel lên trên nó,
tốn vài input token và gần như không tốn độ trễ. Đó là lý do toàn bộ policy đi trong một request,
chứ không phải mỗi hạng mục một lời gọi.

**Chạy lần kiểm input song song với lời gọi model, đừng chạy trước nó.** Kiểm tuần tự thì cộng thẳng
toàn bộ độ trễ của nó vào. Kiểm song song thì gần như không cộng gì, bởi dù sao model của bạn cũng
cần hơn 500ms mới ra được token đầu tiên.

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(my_llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                 # chưa có gì tới tay người dùng
    return safe_response(verdict)
reply = await draft
```

```typescript
const [verdict, draft] = await Promise.all([
  guard.checkInput(message, { session }),
  myLlm.generate(message),
]);
if (!verdict.allowed) return safeResponse(verdict);  // bản nháp bị bỏ
return draft;
```

Cái giá phải trả là token tiêu cho những bản nháp rồi bị bỏ đi. Nếu dưới khoảng 2% số lượt là vi
phạm, chỗ token đó vẫn rẻ hơn phần độ trễ tiết kiệm được. Nếu quy định của bạn nói một prompt vi
phạm tuyệt đối không được chạm tới model, hãy chạy tuần tự và chấp nhận trả độ trễ một cách có ý
thức.

**Stream chậm hơn đúng một chunk.** Câu trả lời dạng stream không thể kiểm trước khi có token đầu
tiên, mà chờ tới token cuối thì không còn là stream nữa. `guard.stream()` cắt tại ranh giới câu, giữ
mỗi chunk lại cho tới khi phần kiểm của nó trả về, và trong lúc đó vẫn để model sinh chunk kế tiếp,
nên chỉ chunk đầu phải trả đủ độ trễ.

```python
async for event in guard.stream(my_llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

Các lần kiểm giữa stream chỉ hỏi những câu sentinel, tức các hạng mục mà bỏ sót là không thể chấp
nhận. Câu trả lời hoàn chỉnh vẫn được hỏi trọn bộ câu hỏi ở cuối, trên event `done`. `chunk_chars`
(mặc định 280) là chỗ đánh đổi giữa số vòng gọi và mức giữ chặt văn bản.

**Bỏ hẳn lời gọi khi có thể.** Trúng cache với nội dung lặp lại, hay trúng prefilter với một ca hiển
nhiên, đều xử lý ngay tại chỗ, không đụng tới mạng.

**Đừng bao giờ để Jev chậm biến thành sự cố của bạn.** Hãy đặt `timeout`, và lưu ý rằng policy đi kèm
fail *open* ở input và fail *closed* ở output (`on_error: {input: fail_open, output: fail_closed}`):
một cú timeout đứng trước con model vốn đã có lớp an toàn riêng thì suy giảm êm ái, còn lần kiểm
output thì chẳng còn gì đứng sau nó nữa. Trong cả hai trường hợp, verdict đều mang `degraded: true`,
nên hãy đếm những ca đó riêng ra.

| Đường đi | Độ trễ cộng thêm |
| --- | --- |
| Trúng prefilter | không, không đụng mạng |
| Trúng cache | không, không đụng mạng |
| Kiểm input, song song với model | gần như không |
| Kiểm input, tuần tự | 70-500ms |
| Câu trả lời dạng stream | chỉ chunk đầu tiên |

**Và trải nghiệm nằm ở `route`, không nằm ở việc chặn.** Một hàng rào chỉ biết từ chối thì với chính
những người nó đang bảo vệ, nó giống một sản phẩm bị lỗi. Vì `route` được quyết riêng khỏi mức nghiêm
trọng, cùng một `review` có thể che số điện thoại rồi vẫn gửi câu trả lời đi, lái câu trả lời sang
hướng khác, hoặc đưa cho người ta nguồn hỗ trợ khủng hoảng - thay vì một câu "tôi không giúp được
việc này" khô khốc. Nối đủ cả sáu route thì phần lớn người dùng không hề nhận ra có hàng rào ở đó.

---

## Đưa lên production

Phần quick start ở trên là code thật, nhưng một hệ thống chạy production cần nhiều hơn ba lời gọi.
Tất cả những thứ dưới đây đều có sẵn trong gói; phần ở trên đã nói kỹ hơn về cache, prefilter,
streaming và các chế độ fail.

- **Cache phán quyết** và **prefilter tất định** - hai cách để một lần kiểm không tốn gì cả.
- **Chế độ fail riêng cho từng bề mặt** - open ở input, closed ở output.
- **Streaming** - nhả văn bản chậm hơn phần kiểm của nó đúng một chunk, để không có gì chưa soát mà
  tới được người dùng.
- **Session** - rủi ro được mang theo giữa các lượt trên cửa sổ hội thoại 10 lượt, nên người vừa
  chạm ngưỡng một hạng mục sẽ bị soi chặt hơn ở lượt kế tiếp.
- **Observer hook** - mọi phán quyết, kể cả ca lấy từ cache và ca degraded, để bạn đẩy vào metrics.

```python
from guardrail_chatbot_jev import Guard, LRUCache

guard = Guard(cache=LRUCache(), observer=metrics.emit, timeout=2.0)
```

**Session phải sống lâu hơn request**, và đây là chỗ một server hay sai một cách âm thầm. Chạy
nhiều worker mà giữ state theo process thì mỗi worker tưởng hội thoại nào cũng vừa mới bắt đầu,
trạng thái theo dõi và các placeholder của lượt bị giữ ngừng được mang theo mà log không nói gì. `Session.as_state()` và `Session.from_state()`
là thứ một store đem đi lưu; [`examples/session_store.py`](examples/session_store.py) có sẵn một
store trong process có giới hạn cho một worker, và một store Redis cho nhiều hơn một.

---

## Example

Tất cả đều chạy được mà không cần API key. Model và verdict lùi về dùng answer đã ghi sẵn, nên mọi
nhánh vẫn thực sự chạy qua.

| | |
| --- | --- |
| [`integration.py`](examples/integration.py) · [`integration.ts`](examples/integration.ts) | Trọn một lượt có hàng rào: check input chạy song song với lời gọi model, streaming, cache, prefilter, session, và cùng một câu hỏi ở bốn thứ tiếng |
| [`chatbot_server.py`](examples/chatbot_server.py) | Đúng lượt đó nhưng đặt sau HTTP: FastAPI, Claude, streaming bằng SSE, session theo từng hội thoại |
| [`domain_policy.py`](examples/domain_policy.py) | Thêm một mảng tuân thủ của riêng bạn lên trên pack có sẵn |

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
PYTHONPATH=python/src python3 examples/domain_policy.py

pip install -e './python[server]'
uvicorn examples.chatbot_server:app --port 8000
```

[`examples/`](examples/) cũng chứa các bộ case có nhãn dùng để hiệu chỉnh: 36 ca input, 15 ca
output và 7 hội thoại, bằng tiếng Anh, Việt, Pháp và Nhật, mỗi ca kèm action mà nó phải cho ra.

## Dòng lệnh

```bash
guardrail-chatbot-jev --surface input --text "làm thuốc nổ kiểu gì" --dry-run
```

`--dry-run` in ra đúng request sẽ được gửi đi, và không cần key. Bỏ nó đi thì exit code mang theo
phán quyết, để script shell rẽ nhánh được:

| Code | Nghĩa |
| --- | --- |
| `0` | allow hoặc flag |
| `1` | redact hoặc guide |
| `2` | review |
| `3` | block |
| `4` | degraded: không gọi được Jev, nên thực tế chưa kiểm gì cả |

`4` tách riêng là có chủ ý. Ở bề mặt input, policy đi kèm fail open, nên action của một verdict
degraded là `allow`; báo đó là thành công thì khác nào nói với `guardrail-chatbot-jev ... && send`
rằng nội dung đã qua một lần kiểm chưa từng chạy.

---

## Tài liệu

Hướng dẫn đầy đủ bao gồm tích hợp, quyền sở hữu policy, hiệu chỉnh ngưỡng và cơ chế đằng sau từng
quyết định.

| | |
| --- | --- |
| [English](docs/guide.md) | [Tiếng Việt](docs/guide.vi.md) |
| [Français](docs/guide.fr.md) | [日本語](docs/guide.ja.md) |

Ngoài ra: [bảng phân loại nguy hại](skill/guardrail-chatbot-jev/references/taxonomy.md), và
[`skill/guardrail-chatbot-jev/`](skill/guardrail-chatbot-jev/), một Claude skill bọc lại đúng những lần kiểm này.

---

## Những gì nó không phải

**Nó không phải lớp cưỡng chế.** Nó trả về phán quyết; hệ thống của bạn quyết định làm gì với phán
quyết đó. Bản thân thư viện không chặn gì cả.

**Jev chỉ đọc đúng những gì bạn đưa cho nó.** Nó không tra cứu được, đếm không đáng tin, không làm
số học, và nó đọc theo nghĩa đen nên phủ định với hàm ý là điểm yếu của nó. Truy xuất dữ liệu, giới
hạn tần suất, trạng thái tài khoản và các phép kiểm tất định là việc của code bao quanh nó.

**Các ngưỡng đi kèm là điểm khởi đầu, không phải kết quả đo đạc.** Chúng được suy ra từ cách một câu
hỏi nhiều lựa chọn phân bổ xác suất. Hãy hiệu chỉnh lại bằng dữ liệu đã gán nhãn của chính bạn trước
khi tin vào chúng trên production; phần hướng dẫn nói rõ cách làm, và [`scripts/`](scripts/) có sẵn
công cụ.

---

## Đóng góp

Rất hoan nghênh issue và pull request. Xem [CONTRIBUTING.md](CONTRIBUTING.md). Cả hai bộ test đều
chạy được mà không cần API key hay mạng, nên xác minh một thay đổi rất dễ:

```bash
cd python && python3 -m pytest -q     # 81 test
cd ts && npm test                     # 57 test
```

`scripts/verify-all.sh` chạy luôn mọi thứ còn lại: policy pack, các bộ case có nhãn, CLI, mọi
example, công cụ tuning offline và phần đóng gói.

Để báo cáo lỗ hổng bảo mật, xem [SECURITY.md](SECURITY.md).

## Giấy phép

[CC BY-NC 4.0](LICENSE): Creative Commons Ghi công - Phi thương mại 4.0 Quốc tế. Bạn được dùng,
chia sẻ và chỉnh sửa cho mục đích phi thương mại, kèm ghi công tác giả. Dùng cho mục đích thương mại
cần được chủ sở hữu bản quyền cho phép riêng.
