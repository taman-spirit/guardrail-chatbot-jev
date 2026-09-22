<h1 align="center">guardrail-chatbot-jev</h1>

<p align="center">
  Kiểm duyệt nội dung cho AI chatbot: soát tin người dùng gửi vào, soát câu bot trả lời ra,<br>
  và nhận về một phán quyết rõ ràng để code của bạn hành động.
</p>

<p align="center">
  <a href="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
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

Một đòn tấn công rải qua nhiều lượt được ghép từ những lượt mà xét riêng lượt nào cũng bào chữa
được. Chính việc xét mỗi lượt từ con số không là thứ khiến nó thành công, nên hàng rào làm thêm hai
việc mà hai lần kiểm đơn lẻ không làm được.

**`check_conversation` đọc toàn bộ transcript.** Đây là bề mặt thứ ba, và nó tìm thứ chỉ lộ ra qua
hình dạng của cả cuộc hội thoại: một đòn crescendo mở đầu vô hại rồi dựa vào chính các câu trả lời
trước của trợ lý, một chuỗi leo thang qua nhiều lượt, một persona đã bị nói cho rời khỏi quy tắc của
chính nó.

**`Session` mang những gì đã xảy ra đi tiếp.** Nó giữ transcript, một điểm rủi ro suy giảm dần, và
một cái sàn đặt dưới vài lượt kế tiếp:

```python
from guardrail_chatbot_jev import Guard, Session

guard = Guard()
session = Session(id=conversation_id)     # mỗi hội thoại một cái, giữ lại giữa các lượt

verdict = guard.check_input(user_message, session=session)
...
session.add_turn("user", user_message)
session.add_turn("assistant", reply)
session.advance()                          # để một cái sàn đã dựng lên được hết hạn

guard.check_conversation(session.history, session=session)
```

Cái sàn mới là phần thực sự đổi phán quyết:

| Cái gì kích hoạt | Sàn nó đặt ra | Kéo dài |
| --- | --- | --- |
| Một verdict **conversation** từ `review` trở lên | `review` | 2 lượt (`carry_turns`) |
| Bất kỳ tin nhắn đơn lẻ nào ra `block` | `flag` | 2 lượt |

Khi sàn còn hiệu lực, một verdict sau đó không thể rơi xuống dưới nó, và route được tính lại cho
khớp, nên một verdict bị nâng sàn không kết thúc bằng `deliver`. Song song, `risk` giảm một nửa mỗi
lượt (`allow` 0, `flag` 0.25, `review` 0.6, `block` 1.0), nên một lượt bị flag sẽ hết ảnh hưởng sau
ba bốn lượt sạch. `session.metadata()` đưa conversation id, số thứ tự lượt và mức rủi ro hiện tại ra
trước mặt Jev ở các lượt sau.

Ba chi tiết đáng biết:

- **Verdict `degraded` không bao giờ làm session dịch chuyển.** Jev không gọi được là sự cố hạ tầng,
  không phải bằng chứng về cuộc hội thoại; tính nó vào sẽ biến một lần outage ngắn thành sự nghi ngờ
  kéo dài với một người dùng vô tội.
- **Sàn do conversation check dựng lên rơi vào lượt kế tiếp, không phải lượt vừa kích hoạt nó.** Đây
  là bản chất chứ không phải đi tắt: cái pattern chỉ nhìn thấy được khi lượt hoàn tất nó đã tồn tại.
  Cho nó chạy ngoài critical path thì người dùng không phải trả thêm gì.
- **Cửa sổ transcript là 10 lượt** (`max_turns`), vì phần leo thang nằm ở các lượt gần nhất và cửa
  sổ ngắn chỉ tốn một phần nhỏ input token. Nâng lên nếu hội thoại của bạn thật sự xây dựng qua
  nhiều lượt hơn.

Session chỉ có tác dụng nếu nó sống lâu hơn request, và đó là bài toán của deployment chứ không phải
của hàng rào; xem [Đưa lên production](#đưa-lên-production) để biết cách lưu nó qua nhiều worker.

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
phần floor ngừng được mang theo mà log không nói gì. `Session.as_state()` và `Session.from_state()`
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

[MIT](LICENSE). Dùng vào việc gì cũng được, kể cả thương mại; chỉ cần giữ lại thông báo bản quyền.
