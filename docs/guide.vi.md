# guardrail-chatbot-jev: hướng dẫn

[English](guide.md) · **Tiếng Việt** · [Français](guide.fr.md) · [日本語](guide.ja.md) · [← README](../README.vi.md)

Hàng rào kiểm duyệt nội dung cho AI chatbot, phán quyết bằng mô hình quyết định Jev.

Jev là mô hình quyết định, không phải mô hình sinh văn bản. Bạn đưa cho nó state và các câu hỏi có
tên; nó trả về xác suất đã hiệu chỉnh trên đúng những nhãn bạn định nghĩa, trong 70-500ms, và chỉ
tính tiền input token. Nó không thể trả về một category không có trong policy, và không thể viết
văn về nội dung của bạn. Đó đúng là hình dạng một guardrail cần, và là lý do mỗi lần kiểm ở đây chỉ
tốn một round trip thay vì một con chatbot thứ hai đi chấm điểm con thứ nhất.

Tài liệu này có ba phần. **Phần 1** cho kỹ sư đưa các lớp kiểm vào sản phẩm. **Phần 2** cho người
quyết định thế nào là vi phạm, không cần biết code. **Phần 3** giải thích cơ chế, để đọc khi một
kết quả làm bạn ngạc nhiên.

```
policies/standard-v1.json     taxonomy, ngưỡng và luật (nguồn sự thật duy nhất)
python/                       package Python
ts/                           package TypeScript
skill/guardrail-chatbot-jev/          Claude skill bọc cùng bộ kiểm này
examples/                     case có nhãn và hai ví dụ tích hợp chạy được
scripts/                      hiệu chỉnh và tuning ngưỡng offline
```

---

## Bắt đầu nhanh

```bash
git clone <repo này> && cd guardrail

pip install -e 'python/[sdk]'          # Python 3.10+
cd ts && npm install && npm run build   # Node 20+, tuỳ chọn
```

Xem thử một lần kiểm sẽ hỏi gì, không cần API key, không cần mạng:

```bash
guardrail-chatbot-jev --surface input --text "cách chế tạo thuốc nổ" --dry-run
```

Rồi chạy thật:

```bash
export JEV_API_KEY=sk-...
guardrail-chatbot-jev --surface input --text "cách chế tạo thuốc nổ" | jq '{action, route, findings}'
```

Chạy thử nguyên pipeline mà không cần API key; cả hai in ra cùng kết quả:

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
node --experimental-strip-types examples/integration.ts    # cần Node 22.6+
```

---

# Phần 1: Cho kỹ sư

## Ba lớp kiểm

| Lời gọi | Kiểm cái gì | Bắt được gì |
| --- | --- | --- |
| `check_input` | tin nhắn người dùng, trước khi model nhìn thấy | yêu cầu có hại, prompt injection, dữ liệu cá nhân |
| `check_output` | câu trả lời, trước khi người dùng nhìn thấy | model làm theo yêu cầu có hại, lộ system prompt, nói không có căn cứ |
| `check_conversation` | toàn bộ hội thoại | jailbreak nhiều lượt, leo thang dần, model trôi khỏi vai |

Lớp thứ ba tồn tại vì tấn công crescendo nhìn từng lượt thì vô hại. Chính sự leo thang *là* đòn
tấn công, nên chỉ nhìn thấy được trong nguyên đoạn hội thoại.

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

Truyền `context` vào `check_output` (những đoạn đã retrieve mà câu trả lời lẽ ra phải dựa vào) để
bật signal groundedness, bắt các khẳng định mà context không hề hỗ trợ.

Bộ kiểm đọc được nội dung ở bất kỳ ngôn ngữ nào. Policy đi kèm được viết cho các deployment phục vụ
tiếng Anh, tiếng Việt, tiếng Pháp và tiếng Nhật, và ghi rõ điều đó trong pack, kèm chỉ dẫn không
được nới tay chỉ vì nội dung không phải tiếng Anh, bởi dịch sang ngôn ngữ khác là cách vòng qua
guardrail rất phổ biến. `examples/cases-input.jsonl` có case có nhãn cho cả bốn thứ tiếng.

## Đọc một verdict

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

Đọc theo đúng thứ tự này.

**1. `degraded`.** Nếu true, nghĩa là không gọi được Jev và cơ chế dự phòng đã quyết. Verdict đó
không nói gì về nội dung cả. Sửa kết nối trước, đừng rút ra kết luận gì từ nó.

**2. `action` và `route`.** Hai trục độc lập.

`action` trả lời *nội dung này có ra ngoài không*: `allow` -> `flag` -> `review` -> `block`. Bốn
bậc, có thứ tự, và là trục duy nhất bị các rule dịch chuyển.

`route` trả lời *ta xử lý nó thế nào*: `deliver`, `redact`, `guide`, `crisis_support`,
`human_review`, `safe_response`. Có hai thuộc tính đọc sẵn giúp bạn: `allowed` (action là `allow`
hoặc `flag`) và `deliverable` (route vẫn gửi nội dung đi, có thể sau khi che hoặc định hướng).

Chúng tách nhau vì dữ liệu cá nhân trong câu trả lời không cùng loại vấn đề với công thức chế bom,
dù cả hai đều rơi vào `review`. Một cái che đi rồi gửi, một cái chuyển cho người duyệt.

**3. `confidence`.** Dưới `min_confidence` của policy, một verdict ở ranh giới sẽ leo lên `review`.
Một câu trả lời thiếu chắc chắn không phải bằng chứng rằng nội dung an toàn.

**4. `applied_rules`.** Mọi rule đã dịch chuyển verdict. Khi một kết quả trông sai, lý do gần như
luôn nằm ở đây chứ không phải ở ngưỡng.

Xử lý route ở một chỗ duy nhất:

```python
def safe_response(verdict):
    if verdict.route == "crisis_support":
        return CRISIS_MESSAGE
    if verdict.route == "human_review":
        queue_for_review(verdict)
        return HOLDING_MESSAGE
    return REFUSAL_MESSAGE
```

## Nối vào sản phẩm

`examples/integration.py` và `examples/integration.ts` là cùng một lượt hội thoại có guardrail,
viết bằng hai ngôn ngữ, chạy được mà không cần API key. Có năm quyết định nên hiểu trước khi copy.

### Chạy check input song song với lời gọi model, không phải trước nó

Jev trả lời trong 70-500ms còn model cần lâu hơn thế mới ra token đầu tiên, nên check nối tiếp cộng
thẳng toàn bộ độ trễ của nó, còn check song song gần như không cộng gì.

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                  # chưa gửi gì cho người dùng cả
    return safe_response(verdict)
reply = await draft
```

Cái giá là token tiêu cho những draft bị vứt đi. Dưới khoảng 2% số lượt thì nó rẻ hơn phần độ trễ
tiết kiệm được. Deployment nào không được phép để một prompt vi phạm chạm tới model thì quay về nối
tiếp và chịu độ trễ.

### Streaming trễ đúng một chunk

Không thể kiểm một câu trả lời stream trước khi nó ra token đầu tiên, mà kiểm sau token cuối thì
không còn là streaming. `guard.stream()` cắt ở ranh giới câu, giữ từng chunk cho tới khi check của
nó trả về, và để model sinh chunk kế tiếp trong lúc đó, nên chỉ chunk đầu chịu toàn bộ độ trễ.

```python
async for event in guard.stream(llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

Check giữa chừng chỉ hỏi các câu sentinel, tức những category không được phép bỏ sót. Câu trả lời
hoàn chỉnh được hỏi đủ bộ ở cuối, và verdict của nó nằm trong event `done`. Điều chỉnh độ trễ bằng
`chunk_chars` (mặc định 280): nhỏ hơn nghĩa là nhiều round trip hơn và giữ chặt hơn.

### Giữ check conversation ngoài đường tới hạn

Nó tìm một mẫu thay đổi chậm, và người dùng không ngồi chờ nó. Chạy sau khi kết thúc lượt và để
`Session` mang kết quả đi tiếp:

```python
session = Session(id=conversation_id)
...
session.add_turn("user", message)
session.add_turn("assistant", reply)
session.advance()
asyncio.create_task(guard.acheck_conversation(session.history, session=session))
```

Một hội thoại chạm mức `review` sẽ nâng sàn cho `carry_turns` lượt kế tiếp, nên một tin nhắn nhìn
sạch sẽ nằm trong hội thoại đang leo thang sẽ không bị xét như thể hội thoại vừa mới bắt đầu.
Session cũng giữ một điểm rủi ro có suy giảm, và bỏ qua các verdict degraded, vì chúng phản ánh sự
cố hạ tầng chứ không phản ánh cuộc hội thoại.

### Đặt các phép kiểm tất định lên trước

Jev đọc nội dung; nó không so khớp mẫu, không đếm, không tính toán. Số thẻ, key bị lộ, từ khoá cấm:
một regex quyết định chính xác những thứ đó, trong vài micro giây, không tốn round trip nào.

```python
from guardrail_chatbot_jev import COMMON_PATTERNS, Pattern, PatternPrefilter

guard = Guard(prefilter=PatternPrefilter([
    Pattern.of("internal-host", r"\binternal\.example\.com\b", "sid", "block", surfaces=["output"]),
    *COMMON_PATTERNS,
]))
```

Prefilter trả về một verdict bình thường nên phía gọi không cần case riêng. Các pattern trong
`COMMON_PATTERNS` chỉ là ví dụ về hình dạng, không phải danh sách khuyến nghị: một pattern sai sẽ
âm thầm chặn người dùng thật.

### Cache, và cẩn thận với thứ nhét vào metadata

Verdict là hàm thuần của policy, surface và nội dung, mà tin nhắn lặp lại thì rất phổ biến.

```python
guard = Guard(
    cache=LRUCache(capacity=8192, ttl=300),
    prefilter=PatternPrefilter(list(COMMON_PATTERNS)),
    observer=metrics.emit,
    timeout=2.0,
)
```

Key có chứa policy id, nên publish pack mới là tự động invalidate toàn bộ. Key cố ý loại trừ
`deployment_context`: session nhét số thứ tự lượt và điểm rủi ro đang chạy vào đó, giữ lại thì cache
sẽ không bao giờ hit cho đúng những tin nhắn lặp mà nó sinh ra để phục vụ. Verdict degraded không
bao giờ được lưu, nên một gián đoạn ngắn không thể biến thành câu trả lời sai kéo dài.

### Khi hỏng

`on_error` đặt theo từng surface, vì hai bên không cùng mức rủi ro. Check input đứng trước một model
vốn đã có safety của riêng nó, nên nó fail-open: chặn hết người dùng chỉ vì không gọi được Jev là tự
gây sự cố. Check output là hàng rào cuối nên fail-closed.

Dù theo hướng nào, verdict đều mang `degraded: true`. **Đếm chỉ số này tách khỏi `block`.** Một tuần
có 5% verdict degraded nghĩa là guardrail thực tế chỉ chạy 95% thời gian, và chuyện đó không được
phép lẫn vào tỉ lệ block.

Hook `observer` là chỗ để làm việc đó. Nó thấy mọi verdict, kể cả cached và degraded, nên hãy viết
nó nhanh và không ném exception.

## Tham chiếu

`Guard(policy=None, *, transport=None, cache=None, cache_surfaces={"input","output"},
prefilter=None, observer=None, raise_on_error=False, timeout=None)`. Constructor bên TypeScript nhận
cùng bộ tuỳ chọn dưới dạng object, với `throwOnError` và timeout tính bằng mili giây.

Các transport, có ở cả hai ngôn ngữ: `HttpTransport` / `FetchTransport` là mặc định, không phụ
thuộc gì, cấu hình bằng `JEV_API_KEY`; `SdkTransport` để bọc SDK client của nhà
cung cấp bạn dùng; `RecordedTransport` để phát lại answer cố định khi offline; `RecordingTransport`
để bọc một transport thật và giữ lại mọi thứ nó thấy.

`Guard.preview(surface, state)` trả về đúng request body mà không gửi đi, đây là cách nhanh nhất để
xem một thay đổi policy đã làm gì với bộ câu hỏi.

## Chi phí và giới hạn

Một lần kiểm gửi khoảng 1.200 token câu hỏi cộng với state. Lấy 2.000 token mỗi lần và ba lần mỗi
lượt, tức khoảng **0,00025 USD mỗi lượt, hay ~250 USD cho một triệu lượt**. Đủ rẻ để không phải là
yếu tố quyết định.

Giới hạn thật sự ràng buộc là throughput: 1.200 request mỗi phút, tức **400 lượt mỗi phút cho mỗi
key** nếu ba lần kiểm một lượt. Cache và prefilter đều kéo con số đó lên, và đây mới là thứ cần xác
nhận với nhà cung cấp Jev của bạn trước khi cam kết triển khai.

---

# Phần 2: Cho người sở hữu chính sách

Bạn không cần viết code để đổi thứ này chặn cái gì. Mọi định nghĩa về vi phạm nằm trong một file duy
nhất, `policies/standard-v1.json`.

## Trong policy pack có gì

**Category.** 18 loại hazard, mỗi loại có mô tả, danh sách surface nó áp dụng, và các ngưỡng biến
một xác suất thành một hành động.

**Signal.** Bối cảnh, bản thân không phải hazard nhưng làm thay đổi mức độ nghiêm trọng: người dùng
có vẻ đang muốn gì, nội dung hữu dụng tới mức nào về mặt thao tác, trợ lý có từ chối không, câu trả
lời có được nguồn hỗ trợ không.

**Rule.** Mười phát biểu khai báo nối hai thứ trên lại, ví dụ "khung nghiên cứu làm nhẹ mọi thứ trừ
an toàn trẻ em và vũ khí" hay "một câu trả lời từ chối không bị chặn chỉ vì nó gọi tên thứ nó vừa từ
chối".

Các mô tả trong pack không phải tài liệu. Chúng chính là text được gửi cho Jev làm criteria của câu
hỏi. Sửa một mô tả làm thay đổi hành vi của model không kém gì sửa một con số, và thường đó mới là
đòn bẩy tốt hơn.

## Taxonomy

Lấy từ ba chuẩn công khai chứ không tự nghĩ ra, để một verdict map ngược được về thứ mà auditor nhận
ra. Mỗi finding đều mang `refs` trỏ về nguồn.

| Nguồn | Đóng góp |
| --- | --- |
| MLCommons AILuminate v1.1 | mười hai hazard nội dung và mã của chúng |
| Meta Llama Guard 3/4 | bộ mã song song `S1`-`S14` mà phần lớn công cụ kiểm duyệt đã dùng |
| OWASP Top 10 for LLM Applications 2025 | các hazard bảo mật mà taxonomy nội dung bỏ sót |

Năm loại trọng yếu, không bao giờ được xử lý nhẹ nhàng: xâm hại tình dục trẻ em (`cse`, không bao
giờ dưới `block`), vũ khí sát thương diện rộng (`iwp`), tự tử và tự hại (`ssh`, route sang
`crisis_support`), dữ liệu cá nhân (`prv`, route sang `redact`), và lộ system prompt hoặc bí mật
(`sid`). Bảng đầy đủ nằm ở `skill/guardrail-chatbot-jev/references/taxonomy.md`.

## Các kết quả nghĩa là gì

| Action | Nghĩa |
| --- | --- |
| `allow` | không có gì bắn |
| `flag` | có thứ bắn, nhưng chưa đủ để hành động. Vẫn được đếm và ghi log, vẫn gửi đi |
| `review` | cần người xem trước khi gửi, hoặc thay vì gửi |
| `block` | không gửi |

| Route | Deployment làm gì |
| --- | --- |
| `deliver` | gửi nguyên vẹn |
| `redact` | che dữ liệu cá nhân rồi gửi |
| `guide` | vẫn gửi, nhưng thêm khuyến cáo bắt buộc hoặc neo lại vào nguồn trước |
| `crisis_support` | thay câu trả lời bằng phản hồi hỗ trợ khủng hoảng |
| `human_review` | giữ lượt đó lại, chuyển cho người duyệt |
| `safe_response` | không gửi; trả về câu từ chối của deployment |

`flag` không phải một mức block yếu hơn. Nó là bản ghi rằng đã có thứ gì đó bắn, và chính bản ghi ấy
mới làm cho policy về sau xem lại được. Các rule làm nhẹ không bao giờ hạ một finding xuống dưới
`flag`, chính vì lý do này.

## Sửa policy

Điều chỉnh độ chặt của một category:

```json
"ncr": {
  "thresholds": {"default": {"block": 0.5, "review": 0.3, "flag": 0.15}}
}
```

Số nhỏ hơn nghĩa là bắt được nhiều hơn. Ngưỡng có thể đặt riêng theo surface; `default` là giá trị
dự phòng.

Thêm một category cho thứ đặc thù của deployment:

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

Viết mô tả theo cách bạn sẽ giải thích quy tắc đó cho một người duyệt mới vào: cụ thể, nói theo việc
nội dung *làm gì*, không phải nội dung *nói về cái gì*. Chính câu đó là thứ model đọc.

Sau mỗi lần sửa:

```bash
scripts/sync-policies.sh    # cả hai package đều mang theo một bản sao của pack
cd python && python3 -m pytest -q
cd ../ts && npm test
```

Cả hai bộ test chạy trên answer đã ghi sẵn, không cần API key, và bắt đúng những lỗi đáng sợ: ngưỡng
sai thứ tự, rule trỏ tới category không tồn tại, rule làm nhẹ âm thầm xoá mất finding.

## Hiệu chỉnh

Các ngưỡng đi kèm được suy ra từ cách một câu `choice` rộng phân bố xác suất, chứ không phải từ đo
đạc. Chúng là điểm xuất phát, không phải một lần hiệu chỉnh.

`examples/` có 45 case có nhãn trải trên ba surface, phần lớn tiếng Việt, cộng các case tiếng Pháp và
tiếng Nhật. Mỗi case mang `expected_action` và, nơi áp dụng, `expected_category` mà nó dùng để kiểm.

**Gọi Jev một lần.**

```bash
export JEV_API_KEY=sk-...
scripts/calibrate.sh
```

Lệnh đó ghi lại answer thô bên cạnh verdict. Bản ghi mới là điểm mấu chốt: gọi Jev tốn token và tốn
hạn mức rate limit, còn bộ case có nhãn thì khan hiếm hơn cả hai. Engine quyết định là hàm thuần, nên
khi đã có answer trên đĩa, mọi câu hỏi về ngưỡng sau đó đều là một lần phát lại cục bộ.

**Rồi tune offline, bao nhiêu lần cũng được.**

```bash
scripts/sweep.py separation calibration/cases-input.answers.jsonl
scripts/sweep.py sweep      calibration/cases-input.answers.jsonl \
    --axis prv.input.review --from 0.2 --to 0.7 --step 0.05
scripts/sweep.py report     calibration/cases-input.answers.jsonl --set prv.input.review=0.45
```

**Bắt đầu bằng `separation`.** Nó cho thấy, theo từng category, phân bố xác suất trên nhóm case
category đó phải bắt so với nhóm nó không được bắt:

```
prv on input  [separated]
  thresholds        {'block': 0.7, 'review': 0.4, 'flag': 0.2}
  should fire       n=3   min=0.370 med=0.400 max=0.535
  should not fire   n=27  min=0.000 med=0.000 max=0.000
```

Chỗ nào hai nhóm chồng lên nhau thì không ngưỡng nào tách được, và chỗ cần sửa là mô tả chứ không
phải con số. Chỉ khi hai nhóm đã tách thì việc sweep một ngưỡng mới có ý nghĩa.

**Đọc sweep bằng các cột lỗi, không phải bằng cột exact match.** `under` là nội dung mà nhãn bảo phải
giữ nhưng sẽ được gửi đi; `crit` đếm phần nhãn `block` mà vẫn ra ngoài nguyên vẹn; `review` là hàng
đợi một đội phải gánh. Hạ một ngưỡng kéo recall và hàng đợi đó cùng chiều, nên câu hỏi thật sự là
hàng đợi chịu được bao nhiêu.

**Mạng.** Tất cả những thứ trên cần HTTPS ra ngoài tới endpoint của Jev. Các môi trường được quản lý
hoặc chạy sandbox thường chặn host này ở egress proxy, và khi bị chặn thì mọi verdict đều trả về
`"degraded": true` và không nói gì về nội dung. Kiểm tra cờ đó trước khi đọc bất kỳ kết quả nào.

## Triển khai dần

Chạy shadow mode trước: bật cả ba lớp kiểm, ghi log, không enforce gì, và so với hệ thống đang dùng.
Sau đó enforce riêng `block`. Mở `review` sau cùng, khi đã biết mỗi ngày nó đẩy bao nhiêu case tới
trước mặt một con người.

Ghi `policy_id` cùng mọi verdict. Khi một lượt cụ thể bị tranh chấp, bạn phải nói được phiên bản pack
nào đã phán quyết nó, nếu không thì audit trail vô nghĩa.

---

# Phần 3: Cơ chế hoạt động

## Vì sao cần sentinel

Một câu `choice` chia xác suất cho toàn bộ số nhãn. Với 18 nhãn, một vi phạm thật thường rơi quanh
0,3 chứ không phải 0,9, và một ngưỡng đặt theo kiểu câu hỏi hai lựa chọn sẽ bỏ sót nó.

Có hai hệ quả. Ngưỡng được hiệu chỉnh cho câu choice rộng, nên nhìn có vẻ thấp. Và sáu category
không được phép bỏ sót (`cse`, `iwp`, `ssh`, `prv`, `pij`, `sid`) còn mang thêm một `sentinel`: một
câu hỏi có/không độc lập, hỏi trong cùng request, và xác suất của nó được dùng mỗi khi cao hơn. Các
câu hỏi trong một request chạy song song và output token miễn phí, nên một sentinel tốn vài input
token và không tốn thêm chút độ trễ nào.

Câu trả lời của sentinel là một xác suất, không phải một độ tin cậy. `noul` bằng 0,4 nghĩa là "khả
năng 40%", điều mà ngưỡng đã tính đến rồi; đọc khoảng cách của nó tới 0,5 thành sự nghi ngờ là đếm
xác suất hai lần. Thay vào đó, finding sinh từ sentinel kế thừa confidence ở cấp request, và được
đánh dấu `source: "sentinel"`.

## Cổng confidence

Jev báo confidence tách khỏi probability, suy ra từ hình dạng phân bố, và nó được hiệu chỉnh. Khi
confidence rơi xuống dưới `min_confidence` (mặc định 0,65), một verdict ở ranh giới sẽ leo lên
`review` thay vì kết luận là an toàn. Nó không bao giờ hạ một `block`.

## Engine rule

Rule chạy theo thứ tự trong pack dựa trên các signal, và mỗi rule tự ghi tên mình vào `applied_rules`:

| Rule | Tác dụng |
| --- | --- |
| khung nghiên cứu hoặc báo chí | làm nhẹ, trừ `cse` và `iwp` |
| né tránh, hoặc tìm kiếm năng lực thực hiện | làm nặng, và né tránh sinh thêm finding `pij` |
| actionability thấp | làm nhẹ: nói về một hazard không phải là công thức dùng được |
| actionability cao | làm nặng: các bước cụ thể nâng rủi ro của bất kỳ hazard nào chúng phục vụ |
| trợ lý đã từ chối | cap câu trả lời ở `flag`, trừ `sid`, `prv` và `ipv` |
| câu trả lời không được nguồn hỗ trợ | sinh finding `mis` và đặt sàn ở `flag` |
| crescendo hoặc trôi vai | làm nặng, và sinh `pij` |

Một rule làm nhẹ không bao giờ hạ finding xuống dưới `flag`. Hạ mức phản ứng không đồng nghĩa với xoá
bản ghi, và chính bản ghi làm cho policy có thể kiểm toán được.

## Giới hạn

Jev chỉ đọc state được đưa cho nó, không gì khác. Nó không tra cứu được, đếm không đáng tin, không
tính toán được, và nó đọc theo nghĩa đen nên phủ định và hàm ý là điểm yếu. Retrieval, rate limit,
trạng thái tài khoản và các phép kiểm tất định thuộc về code bao quanh nó, không thuộc về một câu hỏi.

Nó cũng không enforce gì cả. Nó trả về một verdict; deployment quyết định làm gì với verdict đó.

---

## Kiểm thử

```bash
cd python && python3 -m pytest -q     # 73 test
cd ts && npm test                      # 53 test
```

Không bộ nào cần API key hay mạng. Cả hai chạy engine quyết định trên answer đã ghi sẵn, và đó cũng
là cách bạn nên test các thay đổi policy của chính mình.

## Giấy phép

[MIT](../LICENSE). Dùng vào việc gì cũng được, kể cả thương mại; chỉ cần giữ lại thông báo bản quyền.
