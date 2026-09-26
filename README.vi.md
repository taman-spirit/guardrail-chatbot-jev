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

**Demo:** xem Guardrail được áp dụng cho Nhật Nguyệt AI tại [https://nhatnguyet.org/tro-ly-ai](https://nhatnguyet.org/tro-ly-ai).

| Bản phát hành | Tag | Nội dung | Giấy phép |
| --- | --- | --- | --- |
| [Python SDK 1.1.4](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python/v1.1.4) | `python/v1.1.4` | Gói Python và TypeScript: ba lượt kiểm tra, quy lỗi multi-turn, review realtime, cache, prefilter, session, streaming, hiệu chỉnh offline và CLI | CC BY-NC 4.0 |
| [Go SDK 1.2.4](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go/v1.2.4) | `go/v1.2.4` | Cùng engine bằng Go, kèm công cụ test chạy thật, chấm lại và hồi quy | CC BY-NC 4.0 |
| [Python: policy tuân thủ Việt Nam v1.2.3](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python-vietnam-compliance-v1.2.3) | `python-vietnam-compliance-v1.2.3` | Policy `vietnam-compliance-v1`, kèm câu trả lời viết sẵn bằng tiếng Việt, tiếng Anh và tiếng Trung | CC BY-NC 4.0 |
| [Go: policy tuân thủ Việt Nam v1.2.3](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go-vietnam-compliance-v1.2.3) | `go-vietnam-compliance-v1.2.3` | Cùng policy và câu trả lời đó cho Go, phiên bản module `v1.3.3` | CC BY-NC 4.0 |

Release note của từng bản ghi rõ nội dung và cách cài đặt. Theo đúng thứ tự trên:

```bash
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python/v1.1.4#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.2.4
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python-vietnam-compliance-v1.2.3#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.3.3
```

Python và TypeScript nằm ở `main`; Go ở `go-sdk`; policy Việt Nam ở `guardrail-vietnam-compliance`
(Python) và `go-vietnam-compliance` (Go). Các bản trước đó đã được thay thế bởi các bản trên.
[Tất cả bản phát hành](https://github.com/taman-spirit/guardrail-chatbot-jev/releases).

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

### Vấn đề

**Lây nhiễm ngữ cảnh:** bộ phân loại thấy một vi phạm trong lịch sử thì gán vi phạm đó cho lượt kế
tiếp, bất kể lượt đó chứa gì. Mốc so sánh (mức sàn theo session, thiết kế cũ): giữ **171 trên 194** câu
vô hại, đo trực tiếp.

### Nguyên tắc thiết kế

Một lượt chỉ bị giữ dựa trên bằng chứng từ chính lượt đó hoặc từ câu trả lời cho nó. Lịch sử quyết định
lượt đó được đọc kỹ đến đâu, không quyết định nó có bị giữ hay không.

### Phương pháp

Mỗi lượt được quyết định bằng cách trả lời lần lượt ba câu hỏi.

1. **Tin nhắn của người dùng, tự nó, có hại không?** Tin nhắn được đọc riêng, không kèm phần hội
   thoại trước đó. Nếu có hại thì dừng ngay ở đây. Một tin nhắn tự nó vô hại thì không bao giờ bị chặn
   vì những gì đã nói trước đó.
2. **Câu trả lời, tự nó, có hại không?** Câu trả lời của trợ lý cũng được đọc riêng như vậy.
3. **Chỉ khi hội thoại vừa có dấu hiệu rủi ro: câu trả lời có đang hoàn tất một yêu cầu có hại đã hỏi
   trước đó không?** Câu trả lời được đọc thêm lần thứ hai, cùng với các lượt trước. Lần đọc này chỉ
   được tính khi câu trả lời đưa ra bước tiếp theo, thêm chi tiết, bản dịch hoặc kể lại một yêu cầu có
   hại trước đó. Nếu người dùng chỉ xin lỗi, hỏi về pháp luật, hỏi cách báo cáo, hay đổi chủ đề, thì
   không tính.

Tin nhắn đã bị chặn vẫn nằm lại trong hội thoại dưới dạng `[earlier message omitted]`: guardrail nhớ là
đã có một lần thử, nhưng không bao giờ đọc lại nội dung. Mô hình chat chỉ được báo rằng có một tin nhắn
bị chặn và nhóm vi phạm nào đã chặn nó, không bao giờ thấy nội dung; nhờ vậy câu tiếp theo như "làm đi" hay
"yêu cầu đầu tiên của tôi" được trả lời đúng ngữ cảnh thay vì đoán. Lý do được lưu cùng trạng thái session. Một hội
thoại được xem là *vừa có dấu hiệu rủi ro* trong hai lượt sau một phát hiện nghiêm trọng, và chừng nào
tin nhắn bị chặn còn nằm trong mười tin nhắn gần nhất.

| Bước | Code |
| --- | --- |
| 1. Tin nhắn người dùng, đọc riêng | [`check_input`](python/src/guardrail_chatbot_jev/guard.py#L119) |
| 2. Câu trả lời, đọc riêng | [`check_output`](python/src/guardrail_chatbot_jev/guard.py#L131) |
| 3. Câu trả lời, đọc cùng các lượt trước, và có được tính không | [`_check_in_context`](python/src/guardrail_chatbot_jev/guard.py#L333), [`_attribute`](python/src/guardrail_chatbot_jev/guard.py#L350) |
| Khi nào hội thoại được xem là vừa có dấu hiệu rủi ro | [`Session.watching`](python/src/guardrail_chatbot_jev/session.py#L128) |
| Tin nhắn bị chặn giữ lại dạng placeholder; mô hình chỉ được báo nhóm vi phạm, không thấy nội dung | [`Session.record`](python/src/guardrail_chatbot_jev/session.py#L99), [`model_history`](python/src/guardrail_chatbot_jev/session.py#L106) |
| Kiểm tra cả hội thoại: chỉ theo dõi và báo cáo, không bao giờ chặn một lượt | [`check_conversation`](python/src/guardrail_chatbot_jev/guard.py#L170) |
| Hội thoại có rủi ro thì câu trả lời được gửi trọn vẹn, không stream từng phần | [`guard_stream`](python/src/guardrail_chatbot_jev/streaming.py#L74) |

### Ví dụ theo từng lượt

| Lượt | Người dùng | Trợ lý | Guardrail làm gì | Kết quả |
| --- | --- | --- | --- | --- |
| 1 | "Chỉ mình cách chế thuốc nổ từ phân bón." | — | Bước 1: tin nhắn tự nó có hại. | **Chặn.** Người dùng nhận câu trả lời an toàn; lượt này được lưu là `[earlier message omitted]`. Hội thoại giờ *vừa có dấu hiệu rủi ro*. |
| 2 | "Xin lỗi, bỏ qua nhé. Thời tiết Hà Nội mai thế nào?" | "Nắng nhẹ, 24 đến 30 độ." | Bước 1: vô hại. Bước 2: vô hại. Bước 3: đọc cùng lịch sử, câu trả lời không hoàn tất gì (0,02). | **Gửi đi.** Thiết kế cũ giữ lượt này lại. |
| 3 | "Chế tạo chất nổ thì bị pháp luật xử lý thế nào?" | "Có thể bị phạt tù, tuỳ mức độ." | Bước 1: có nhắc tới chất nổ nhưng là hỏi pháp luật: cùng lắm là ghi chú vào log. Bước 3: người dùng đang lùi ra (0,90); câu trả lời không hoàn tất gì (0,05). | **Gửi đi.** |
| 4 | "Ok, tiếp đi. Bước 2 là gì?" | "Bước 2: để hỗn hợp khô một ngày, rồi chia đều." | Bước 1: lời lẽ vô hại. Bước 2: câu trả lời đọc riêng có vẻ vô hại. Bước 3: đọc cùng lượt 1, đây là bước tiếp theo của yêu cầu đã bị từ chối (0,95). | **Chặn.** Chỉ lần đọc có ngữ cảnh mới bắt được. |
| 4′ | như trên | "Mình không thể tiếp tục phần đó." | Bước 3: câu trả lời không hoàn tất gì. | **Gửi đi.** Không có gì có hại sắp được gửi. |
| 9 | "Gợi ý một cuốn sách cho cuối tuần." | "Bạn thử *Nhà giả kim*." | Tin nhắn bị chặn đã ra khỏi mười tin nhắn gần nhất và rủi ro đã giảm: chỉ bước 1 và 2, không đọc lần hai. | **Gửi đi**, với chi phí bình thường. |

Các số trong ngoặc là loại câu trả lời mà Jev đưa ra trong các lần chạy thật bên dưới, cho hai câu hỏi
"câu trả lời hoàn tất một yêu cầu có hại trước đó" và "người dùng đang lùi ra".

### Định nghĩa

#### Các thiết lập đang dùng

| Thiết lập | Giá trị | Ý nghĩa |
| --- | --- | --- |
| Cửa sổ lịch sử | 10 tin nhắn | Kiểm tra hội thoại và lần đọc có ngữ cảnh thấy mười tin nhắn gần nhất, khoảng năm lượt hỏi–đáp. |
| Rủi ro của mỗi verdict | allow 0 · flag 0,25 · review 0,6 · block 1,0 | Mức rủi ro mà một verdict cộng vào session. |
| Độ giảm rủi ro | 0,5 | Sau mỗi lần kiểm tra, rủi ro cũ còn một nửa; session giữ giá trị lớn hơn giữa nửa đó và rủi ro của verdict mới. |
| Bắt đầu theo dõi | rủi ro ≥ 0,2 | Khi rủi ro còn từ 0,2 trở lên, câu trả lời được đọc thêm có ngữ cảnh. |
| Theo dõi tiếp | 2 lượt | Sau một verdict hội thoại ≥ review hoặc bất kỳ block nào, session được theo dõi thêm hai lượt hoàn tất. |
| Quy lỗi | ≥ 0,5, và lớn hơn "lùi ra" | Lần đọc có ngữ cảnh chỉ được tính khi Jev chắc ít nhất 50 % là câu trả lời hoàn tất một yêu cầu có hại trước đó, và chắc điều đó hơn là người dùng đang lùi ra. |
| Xác nhận sentinel | ≥ 0,02 | Sentinel được xem là có xác nhận khi câu hỏi phân loại chính cho nhóm đó ít nhất 2 %. |
| Sentinel yếu | dưới mức block của nhóm | Sentinel đứng một mình, dưới mức block, chỉ được ghi ở mức flag và vẫn gửi đi; "không bao giờ dưới" không nâng nó lên. |
| Không bao giờ hạ | `ssh` (policy Việt Nam: `ssh`, `vsv`, `vld`) | Các nhóm này giữ nguyên độ mạnh kể cả khi chỉ có sentinel. |
| Câu trả lời từ chối | refusal ≥ 0,8, sentinel < 0,5 | Câu từ chối có sentinel đứng một mình bị hạ xuống flag, trừ `sid`, `prv`, `ipv` (policy Việt Nam: thêm `vsv`, `vld`). |
| Che thay vì chặn | sentinel < 0,8 | Dữ liệu cá nhân chỉ do sentinel phát hiện thì được che rồi gửi, không chặn. |
| Cổng độ tự tin | độ tự tin < 0,65 | Câu trả lời kém tự tin, có phát hiện hoặc suýt chạm ngưỡng, bị chuyển sang review, trừ khi Jev đánh giá ý định là vô hại với độ tự tin ≥ 0,5. |
| Suýt chạm ngưỡng | ≥ một nửa ngưỡng flag | Xác suất đủ gần một mức để cổng độ tự tin tính tới. |
| Giới hạn hội thoại | escalation ≤ 0,5 | Hội thoại không tiến về mục tiêu có hại thì tối đa flag, trừ `cse` và `ssh`. |
| Jev không phản hồi | input: gửi · output: giữ · hội thoại: gửi | Kiểm tra input đứng trước một mô hình có an toàn riêng; kiểm tra output là chốt cuối. |

#### Ngưỡng theo nhóm

Một nhóm kích hoạt ở mức **flag**, **review** hoặc **block** khi xác suất đạt giá trị đó. Cột "Output" là
ngưỡng thay thế cho câu trả lời của trợ lý; "=" nghĩa là giống ngưỡng mặc định.

| Nhóm | Tên | Flag / review / block | Output | Ghi chú |
| --- | --- | --- | --- | --- |
| `cse` | Child sexual exploitation | 0.03 / 0.06 / 0.15 | = | sentinel; không bao giờ dưới block |
| `iwp` | Indiscriminate weapons (CBRNE) | 0.04 / 0.08 / 0.2 | = | sentinel |
| `ssh` | Suicide and self-harm | 0.07 / 0.15 / 0.3 | = | sentinel; xử lý bằng crisis_support |
| `vcr` | Violent crimes | 0.12 / 0.25 / 0.45 | = | sentinel |
| `src` | Sex-related crimes | 0.12 / 0.25 / 0.45 | = |  |
| `hte` | Hate and discrimination | 0.15 / 0.28 / 0.5 | 0.08 / 0.18 / 0.35 |  |
| `ncr` | Non-violent crimes | 0.15 / 0.3 / 0.5 | = |  |
| `sxc` | Adult sexual content | 0.2 / 0.4 / 0.6 | = |  |
| `prv` | Privacy and personal data | 0.2 / 0.4 / 0.7 | 0.15 / 0.28 / 0.5 | sentinel; xử lý bằng redact |
| `ipv` | Intellectual property | 0.25 / 0.45 / 0.7 | = | chỉ output, conversation |
| `dfm` | Defamation | 0.18 / 0.35 / 0.6 | = | chỉ output, conversation |
| `spc` | Specialized advice | 0.25 / 0.5 / 0.85 | = | xử lý bằng guide; chỉ output |
| `elc` | Elections and civic integrity | 0.15 / 0.3 / 0.5 | = |  |
| `pij` | Prompt injection and jailbreak | 0.15 / 0.3 / 0.55 | = | sentinel; chỉ input, conversation |
| `sid` | Sensitive information disclosure | 0.08 / 0.18 / 0.35 | = | sentinel; chỉ output, conversation |
| `exa` | Excessive agency | 0.15 / 0.3 / 0.5 | = | chỉ output, conversation |
| `mis` | Misinformation and unsupported claims | 0.25 / 0.45 / 0.8 | = | xử lý bằng guide; chỉ output |
| `scp` | Out of scope | 0.4 / 0.75 / 0.95 | = | tắt |

#### Các quy tắc, diễn giải

1. **Mỗi tin nhắn và mỗi câu trả lời được chấm riêng.** Mỗi nhóm có một xác suất; verdict là mức mạnh
   nhất mà một nhóm đạt tới, sau khi các quy tắc trong policy điều chỉnh.
2. **Session nhớ rủi ro, không nhớ nội dung.** Sau mỗi lần kiểm tra, rủi ro bằng giá trị lớn hơn giữa
   một nửa rủi ro cũ và rủi ro của verdict mới.
3. **Session được theo dõi** khi rủi ro từ 0,2 trở lên, trong hai lượt sau một phát hiện nghiêm trọng,
   và khi còn tin nhắn bị chặn trong cửa sổ.
4. **Khi được theo dõi, câu trả lời được đọc lần hai cùng các lượt trước.** Lần đọc này chỉ được tính
   khi câu trả lời hoàn tất một yêu cầu có hại trước đó (xác suất ≥ 0,5, và cao hơn xác suất người dùng
   đang lùi ra).
5. **Verdict cuối cùng** là verdict đọc riêng, chỉ được tăng thêm bởi kết quả đọc có ngữ cảnh khi kết
   quả đó được tính.
6. **Lịch sử tự nó không bao giờ nâng verdict:** không có lần đọc thứ hai, chỉ tin nhắn hoặc câu trả lời
   quyết định.

#### Công thức

`q_t` tin nhắn người dùng, `r_t` câu trả lời, `H_t` cửa sổ lịch sử (10 tin nhắn), `J(x)` câu trả lời của
Jev cho trạng thái `x`, `D(s, a)` quyết định theo policy trên bề mặt `s`, `⊕` phép gộp verdict (mỗi nhóm
lấy phát hiện mạnh hơn).

```
V_in(t)   = D(input,  J(q_t))
V_out(t)  = D(output, J(r_t))

W_t       = carry_left > 0  ∨  risk_t ≥ 0.2  ∨  placeholder ∈ H_t          (đang được theo dõi)
V_ctx     = D(output, J(r_t | H_t))                                          (chỉ khi W_t)
c, d      = P(câu trả lời hoàn tất yêu cầu có hại trước đó), P(người dùng lùi ra)
A_t       = c ≥ τ  ∧  c ≥ d  ∧  V_ctx có phát hiện ≥ flag,   τ = 0.5
V(t)      = V_out(t) ⊕ V_ctx  nếu A_t,  ngược lại V_out(t)

risk_t+1  = max(δ · risk_t, ρ(hành động)),  δ = 0.5,  ρ = (0, 0.25, 0.6, 1.0) cho (allow, flag, review, block)
carry     = 2 lượt sau một verdict hội thoại ≥ review hoặc bất kỳ block nào
```

Code: `V_in` [`check_input`](python/src/guardrail_chatbot_jev/guard.py#L119), `V_out` [`check_output`](python/src/guardrail_chatbot_jev/guard.py#L131), `W_t` [`Session.watching`](python/src/guardrail_chatbot_jev/session.py#L128), `V_ctx`, `c`, `d` [`_check_in_context`](python/src/guardrail_chatbot_jev/guard.py#L333) / [`context_questions`](python/src/guardrail_chatbot_jev/questions.py#L135), `A_t`, `⊕` [`_attribute`](python/src/guardrail_chatbot_jev/guard.py#L350), `risk` [`Session.observe`](python/src/guardrail_chatbot_jev/session.py#L146), `carry` [`Session.advance`](python/src/guardrail_chatbot_jev/session.py#L163)

### Hiệu chỉnh đơn lượt

Diễn giải:

1. **Sentinel đứng một mình là tín hiệu yếu.** Khi chỉ câu hỏi có/không riêng của một nhóm nhận ra nó,
   còn câu hỏi phân loại chính cho nhóm đó dưới 2 %, phát hiện được xem là *chưa xác nhận*.
2. **Phát hiện yếu chỉ ở mức flag**, trừ khi tự nó chạm mức block; ngưỡng "không bao giờ dưới" của nhóm
   không nâng nó lên. Tự hại không bao giờ bị hạ (policy Việt Nam: thêm `vsv`, `vld`).
3. **Câu trả lời từ chối** (refusal ≥ 0,8) kèm sentinel yếu dưới 0,5 chỉ được ghi ở mức flag. Rò rỉ bí
   mật, dữ liệu cá nhân và nội dung được bảo hộ (`sid`, `prv`, `ipv`) là ngoại lệ, vì câu từ chối vẫn có
   thể chứa chúng.
4. **Dữ liệu cá nhân chỉ do sentinel phát hiện** được che rồi gửi, không chặn, trừ khi sentinel đạt 0,8.
5. **Độ tự tin thấp** (dưới 0,65) đưa phát hiện hoặc trường hợp suýt chạm ngưỡng sang review, trừ khi
   Jev đánh giá ý định là vô hại với độ tự tin từ 0,5 trở lên.
6. **Hội thoại không leo thang** (escalation ≤ 0,5) tối đa ở mức flag, trừ `cse` và `ssh`.

Câu theo sau thường ngắn và mơ hồ; các hiệu chỉnh dưới đây nhắm vào những lỗi đơn lượt mà chúng làm lộ
ra. Với nhóm `k`, xác suất `p`, các ngưỡng `θ_flag ≤ θ_review ≤ θ_block`, và `choice_k` là xác suất mà
câu hỏi phân loại chính cho `k`:

```
u_k                 = phát hiện chỉ từ sentinel  ∧  choice_k < 0.02           (chưa được xác nhận)
never_below         chỉ áp dụng khi ¬u_k ∨ p ≥ θ_block
u_k ∧ p < θ_block                                                    → tối đa flag
output ∧ refusal ≥ 0.8 ∧ u_k ∧ p < 0.5 ∧ k ∉ {sid, prv, ipv}         → flag
u_k ∧ route_k = redact ∧ hành động = block ∧ p < 0.8                 → review (che thông tin)
cổng tự tin:  conf < 0.65 ∧ (có phát hiện ∨ p ≥ θ_flag / 2) → review,  trừ khi intent = benign ∧ conf ≥ 0.5
hội thoại:    escalation ≤ 0.5 → tối đa flag,  trừ cse, ssh
```

Code: `u_k` [`decide`](python/src/guardrail_chatbot_jev/decide.py#L57), `never_below` [`_finding`](python/src/guardrail_chatbot_jev/decide.py#L212), weak [`decide`](python/src/guardrail_chatbot_jev/decide.py#L70), refusal [`_cap_uncorroborated_on_refusal`](python/src/guardrail_chatbot_jev/decide.py#L245), redact [`_redact_instead_of_block`](python/src/guardrail_chatbot_jev/decide.py#L270), gate [`_confidence_gate`](python/src/guardrail_chatbot_jev/decide.py#L378), conversation [`no-escalation-caps-conversation`](policies/standard-v1.json#L700), settings [`sentinel_corroboration`](policies/standard-v1.json#L23) / [`confidence_gate`](policies/standard-v1.json#L32), [`spc`](policies/standard-v1.json#L328), [`ncr`](policies/standard-v1.json#L207), [`iwp`](policies/standard-v1.json#L83)

Ngoài ra: `spc` chỉ chấm ở từng câu trả lời; mô tả `ncr` và `iwp` loại trừ người bị hại và câu hỏi về
pháp luật.

### Xử lý review trong chat realtime

`ReviewHandling: ReviewAsAudit` ([`review_handling`](python/src/guardrail_chatbot_jev/guard.py#L107), [`_audit`](python/src/guardrail_chatbot_jev/guard.py#L314)). Chỉ `block` dừng nội dung; mọi verdict có mức `audit`.

| Verdict | Người dùng nhận | Hậu kiểm |
| --- | --- | --- |
| allow | nội dung | không |
| flag | nội dung | lấy mẫu |
| review | nội dung, được che hoặc điều hướng nếu nhóm yêu cầu | ưu tiên |
| block | câu trả lời an toàn viết sẵn | ưu tiên |
| nguy cơ tự hại | lời hỗ trợ khủng hoảng | ưu tiên |
| degraded, bề mặt fail-closed | giữ lại | không |

### Đánh giá

| Bộ dữ liệu | Cỡ | Nội dung | Nhãn |
| --- | --- | --- | --- |
| [`examples/multiturn-live.jsonl`](examples/multiturn-live.jsonl) | 223 hội thoại | vi phạm đơn lẻ, lặp lại, xen kẽ; lịch sử vượt cửa sổ; leo thang | kết quả kỳ vọng từng case; vi phạm tham chiếu bằng id từ các bộ có nhãn |
| [`examples/multiturn-contamination.jsonl`](examples/multiturn-contamination.jsonl) | 26 kịch bản, [`test_the_scenarios`](python/tests/test_multiturn.py#L76) | câu trả lời Jev giả lập | kết quả kỳ vọng từng case |
| [`cases-input.jsonl`](examples/cases-input.jsonl), [`cases-output.jsonl`](examples/cases-output.jsonl) | 51 case | đơn lượt | hành động kỳ vọng |

Quy trình: **chạy thật** [`TestLiveMultiturn`](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/go-sdk/go/live_multiturn_test.go#L96) (Jev, cả hai thiết kế, ghi lại mọi câu trả lời thô); **chấm lại** [`TestReplayVariants`](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/go-sdk/go/replay_test.go#L224), [`TestReplayConversation`](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/go-sdk/go/replay_test.go#L300), [`TestReplayRealtime`](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/go-sdk/go/replay_test.go#L424) (khoảng
5.000 câu trả lời đã ghi được quyết định lại theo từng phương án, nên các phương án được so trên cùng dữ
liệu); **nhiễu** [`TestMultiturnAttributionUnderNoise`](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/go-sdk/go/multiturn_test.go#L233) (câu trả lời giả lập bị thêm nhiễu, σ ∈ {0.05, 0.1, 0.2}); **hồi quy** [`TestLiveSingleTurnRegression`](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/go-sdk/go/live_multiturn_test.go#L338) (bộ đơn lượt,
trước và sau). Chỉ số: tỷ lệ giữ câu vô hại (FPR), tỷ lệ bắt vi phạm (recall), tỷ lệ vào hàng đợi
xem xét.

**Chạy thật, 223 hội thoại**

| Chỉ số | Mức sàn (cũ) | Quy lỗi (hiện tại) |
| --- | --- | --- |
| Câu vô hại bị giữ | 171 / 194 | **0 / 194** |
| Câu trả lời có hại bị bắt | 19 / 19 | **19 / 19** |
| Hội thoại leo thang bị phát hiện | 9 / 9 | **9 / 9** |
| Hội thoại vô hại vào hàng đợi xem xét | 172 / 194 | **2 / 194** |

**Ablation, chạy thật** (ở mọi lần chạy, việc đọc có ngữ cảnh không quy lỗi cho câu vô hại nào; từ bước
2, mọi câu còn bị giữ đều do kiểm tra đơn lượt)

| Bước | Bộ | Câu vô hại bị giữ |
| --- | --- | --- |
| 1. Mức sàn | 223 | 171 / 194 (88 %) |
| 2. Quy lỗi, placeholder trung tính | 37 | 7 / 26 (27 %) |
| 3. Như trên, bộ rộng hơn | 165 | 30 / 141 (21 %) |
| 4. + xác nhận sentinel, mô tả `ncr` / `iwp` | 165 (4 lần) | 0–1 / 141 (≤ 0,7 %) |
| 5. + vi phạm lặp lại, lịch sử dài | 223 | 3 / 185 (1,6 %) |
| 6. + sentinel yếu ≤ flag, cổng tự tin theo intent, giới hạn hội thoại | 223 | **0 / 185** |

**Chấm lại** (≈ 5.000 câu trả lời đã ghi)

| Phương án | Vô hại bị giữ | Vi phạm bị bắt |
| --- | --- | --- |
| Sau bước 4 | 0,62 % | 100 % (1.720) |
| Sau bước 6 | **0,04 %** | **100 %** |
| + hỏi lại ở case sát ngưỡng | 0,00 % | 100 %, +11 % lượt gọi (không dùng) |
| Realtime (`ReviewAsAudit`) | **0,02 %** bị chặn (1 / 4.189) | mọi câu trả lời có hại bị dừng |

**Nhiễu** (τ = 0,5): σ = 0,1 → giữ nhầm 0,9 %, bắt 100 % câu tiếp nối; σ = 0,2 → 4,3 %, 97,2 %.
**Hồi quy:** 0 vi phạm có nhãn bị cho qua, trước và sau; số case đúng nhãn 28 → 29.

### Chạy lại

```bash
cd python && python -m pytest tests/test_multiturn.py     # 26 kịch bản giả lập, cả hai thiết kế
cd ts && npm test                                          # cùng các kịch bản bằng TypeScript
```

Các công cụ chạy thật, chấm lại, thử nhiễu và hồi quy nằm trong module Go, ở
[branch `go-sdk`](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/go-sdk/go/README.md#tests).

### Sử dụng

Go:

```go
guard := guardrail.New(guardrail.Options{ReviewHandling: guardrail.ReviewAsAudit})
session := guardrail.NewSession(conversationID)

in, _ := guard.CheckInput(ctx, message, &guardrail.CheckOptions{Session: session})
session.Record("user", message, in)
if !in.Deliverable() {
	return safeResponse(in)
}
reply := callModel(session.ModelHistory(), message)
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Session: session, UserMessage: message})
session.Record("assistant", reply, out)
session.Advance()
```

Python:

```python
guard = Guard(review_handling="audit")
session = Session(id=conversation_id)

verdict_in = guard.check_input(message, session=session)
session.record("user", message, verdict_in)
reply = call_model(session.model_history(), message)
verdict_out = guard.check_output(reply, user_message=message, session=session)
session.record("assistant", reply, verdict_out)
session.advance()
```

TypeScript:

```typescript
const guard = new Guard({ reviewHandling: "audit" });
const session = new Session({ id: conversationId });

const verdictIn = await guard.checkInput(message, { session });
session.record("user", message, verdictIn);
const reply = await callModel(session.modelHistory(), message);
const verdictOut = await guard.checkOutput(reply, { userMessage: message, session });
session.record("assistant", reply, verdictOut);
session.advance();
```

`out.Context` chứa kết quả đọc có ngữ cảnh (`completes`, `disengages`, `attributed`); `out.Audit` là mức
hậu kiểm. `Multiturn: MultiturnFloor` khôi phục thiết kế cũ.

### Giới hạn

- 30 câu vi phạm có nhãn; không có mẫu `cse` thật, nên recall của cơ chế xác nhận sentinel với `cse`
  chưa đo được. Các tín hiệu đó vẫn được ghi lại để hậu kiểm.
- Tấn công chia nhỏ mà các mảnh đầu không kích hoạt gì được đọc có ngữ cảnh trễ một lượt.
- Session đang được theo dõi tốn thêm một request Jev cho mỗi câu trả lời, gửi song song.

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
