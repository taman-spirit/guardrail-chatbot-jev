# Hướng dẫn tuân thủ cho dịch vụ AI tại Việt Nam

**Tiếng Việt** · [English](vietnam-compliance.md) · [中文](vietnam-compliance.zh.md)

Hướng dẫn này mô tả từng bước đưa một chatbot AI vào vận hành tại Việt Nam với policy
`vietnam-compliance-v1`, theo hai văn bản:

- **Luật Trí tuệ nhân tạo**
- **Luật An ninh mạng**

> Đây là hướng dẫn kỹ thuật, không phải tư vấn pháp lý. Hãy đối chiếu với văn bản luật và các văn
> bản hướng dẫn thi hành đang có hiệu lực, và để bộ phận pháp chế của bạn duyệt trước khi vận hành.

---

## Bước 1. Xác định phạm vi và người chịu trách nhiệm

1. Liệt kê mọi điểm mà nội dung đi vào và đi ra khỏi mô hình: tin nhắn người dùng, câu trả lời,
   toàn bộ hội thoại, nội dung truy xuất (RAG).
2. Mỗi điểm tương ứng với một lần kiểm tra: `input`, `output`, `conversation`.
3. Chỉ định một người chịu trách nhiệm về policy: người duyệt thay đổi ngưỡng, duyệt các câu trả lời
   viết sẵn và nhận báo cáo định kỳ.

## Bước 2. Minh bạch rằng người dùng đang nói chuyện với AI (Luật Trí tuệ nhân tạo)

1. Thông báo rõ cho người dùng rằng họ đang tương tác với một hệ thống AI, ngay từ đầu cuộc trò
   chuyện.
2. Gắn nhãn cho nội dung do AI tạo ra khi nội dung đó được đưa ra ngoài phạm vi cuộc trò chuyện.
3. Không để trợ lý tự nhận là con người. Nhóm `vai` kiểm tra câu trả lời có tự nhận là người khi
   người dùng hỏi thật lòng hay không.

## Bước 3. Ngăn nội dung bị cấm

Policy chia nội dung vi phạm thành các nhóm, mỗi nhóm có câu trả lời riêng:

| Nhóm | Căn cứ | Guardrail chặn | Guardrail **không** chặn |
| --- | --- | --- | --- |
| `vsv` Chủ quyền lãnh thổ | Luật An ninh mạng | Nội dung phủ nhận, xuyên tạc chủ quyền lãnh thổ của Việt Nam | Thời tiết, du lịch, lịch sử, tin tức, câu hỏi về địa vị pháp lý |
| `vas` Chống phá Nhà nước | Luật An ninh mạng | Tuyên truyền chống Nhà nước, kích động lật đổ, xuyên tạc lịch sử | Câu hỏi về thể chế, pháp luật, chính sách; góp ý hợp pháp |
| `vld` Lãnh tụ, lãnh đạo, biểu tượng quốc gia | Luật An ninh mạng | Xúc phạm, bịa đặt về lãnh tụ, lãnh đạo, anh hùng dân tộc, Quốc kỳ, Quốc huy, Quốc ca | Tiểu sử, chức danh, trích dẫn, tin tức |
| `vcs` Tin giả, gây rối | Luật An ninh mạng | Tin giả gây hoang mang, kích động gây rối, tấn công hệ thống thông tin | Hỏi kiểm chứng tin đồn, báo cáo tin giả, kiến thức phòng thủ |
| `prv` Thông tin cá nhân | Luật An ninh mạng | Dữ liệu cá nhân của **cá nhân**: số căn cước, địa chỉ nhà, số điện thoại riêng | Hotline, số chăm sóc khách hàng, email hỗ trợ, địa chỉ, mã số thuế của **tổ chức** |
| `vai` Dùng AI để lừa dối | Luật Trí tuệ nhân tạo | Deepfake, giả giọng, mạo danh, thao túng người yếu thế | Giải thích AI, nội dung có ghi rõ do AI tạo |

Các nhóm an toàn chung (bạo lực, vũ khí, xâm hại trẻ em, tự hại...) được giữ nguyên từ
`standard-v1`.

## Bước 4. Tích hợp vào ứng dụng

Python:

```python
from guardrail_chatbot_jev import Guard, Responder, Session, detect_language

guard = Guard("vietnam-compliance-v1")
responder = Responder(guard.policy)          # đường dây hỗ trợ mặc định: 115

def handle(session: Session, message: str) -> str:   # một session cho mỗi cuộc hội thoại
    lang = detect_language(message)          # "vi", "en" hoặc "zh"
    verdict_in = guard.check_input(message, session=session)
    session.record("user", message, verdict_in)
    if held := responder.blocking_response([verdict_in], language=lang):
        session.advance()
        return held
    reply = call_model(responder.model_history(session, lang))   # lượt bị chặn được thay bằng ghi chú nêu nhóm
    verdict_out = guard.check_output(reply, user_message=message, session=session)
    sent = responder.compose(reply, [verdict_in, verdict_out], language=lang)
    session.record("assistant", sent, verdict_out)
    session.advance()
    return sent
```

Go (có trong bản phát hành `go-vietnam-compliance-v1`, module `v1.1.0` trở lên):

```go
policy, _ := guardrail.BundledPolicy("vietnam-compliance-v1")
guard := guardrail.New(guardrail.Options{Policy: policy})
responder, _ := guardrail.NewResponder(policy, "") // "" = 115
session := guardrail.NewSession(conversationID) // một session cho mỗi cuộc hội thoại

lang := guardrail.DetectLanguage(message)
in, _ := guard.CheckInput(ctx, message, &guardrail.CheckOptions{Session: session})
session.Record("user", message, in)
if held, ok := responder.BlockingResponse([]guardrail.Verdict{in}, lang); ok {
	session.Advance()
	return held
}
reply := callModel(responder.ModelHistory(session, lang)) // lượt bị chặn được thay bằng ghi chú nêu nhóm
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Session: session, UserMessage: message})
sent := responder.Compose(reply, []guardrail.Verdict{in, out}, lang)
session.Record("assistant", sent, out)
session.Advance()
return sent
```

## Bước 5. Dùng câu trả lời viết sẵn, không để mô hình tự viết

1. Mỗi nhóm vi phạm có một câu trả lời riêng bằng tiếng Việt, tiếng Anh và tiếng Trung, lưu trong
   phần `responses` của policy. `Responder` chỉ chọn, không sinh văn bản.
2. Câu trả lời khi vi phạm khẳng định dịch vụ hoạt động tuân thủ pháp luật Việt Nam và gợi ý một
   hướng hỏi hợp lệ.
3. Mọi trường hợp liên quan đến chủ quyền đều kết thúc bằng đoạn khẳng định viết sẵn trong policy,
   giữ nguyên từng ký tự. Mô hình không tự viết đoạn này, để tránh sai số hiệu văn bản.
4. Khi người dùng có dấu hiệu tự hại, câu trả lời mang tính thấu cảm, không nhắc đến pháp luật, và
   hướng dẫn gọi **115**. Nếu tổ chức của bạn có đường dây hỗ trợ tâm lý đã được xác minh, truyền vào
   `Responder(policy, crisis_line="...")`.
5. Nội dung đang chờ nhân viên xem xét nhận thông báo trung tính, không kết luận người dùng vi phạm.
6. Khi hệ thống kiểm duyệt gián đoạn, người dùng nhận thông báo gián đoạn, không bị buộc tội.
7. Câu trả lời nêu đúng điều đã khiến nội dung bị chặn: bạo lực và vũ khí, xâm hại trẻ em, tội
   phạm, nội dung khiêu dâm, thù ghét, vượt qua thiết lập an toàn, bản quyền, cùng các nhóm theo pháp
   luật Việt Nam. Câu trả lời khủng hoảng chỉ dùng khi tự hại là mối lo **dẫn đầu**. Một yêu cầu chế
   tạo thứ phá hủy tòa nhà nhận câu trả lời về bạo lực; nếu tín hiệu tự hại vẫn vượt ngưỡng chặn, câu
   trả lời có thêm một dòng hướng dẫn gọi **115**.
8. Sau một lượt bị chặn, gửi cho mô hình `responder.model_history(session, lang)` (Go: `responder.ModelHistory`) thay vì lịch sử
   thô. Lượt bị chặn được thay bằng một ghi chú nêu nhóm vi phạm (không bao giờ chứa nội dung gốc) và
   câu trả lời người dùng đã nhận. Nhờ vậy, khi người dùng nói "làm đi" hay "yêu cầu đầu tiên của
   tôi", mô hình biết mình đã từ chối điều gì, thay vì đoán hoặc nói không thấy tin nhắn. Nhóm vi phạm được lưu trong `session.as_state()`, nên vẫn đúng khi session được lưu vào store rồi khôi phục bằng `Session.from_state()`.

## Bước 6. Không chặn nhầm câu hỏi bình thường

Chặn nhầm cũng là lỗi. Policy có hai cơ chế giảm chặn nhầm:

1. **Nhắc đến một cách trung tính.** Khi nội dung chỉ nhắc đến một địa danh, một người hay một tổ
   chức (thời tiết, du lịch, chức danh, tin tức), kết quả bị giới hạn ở mức ghi nhận: không bị chặn,
   không bị giữ lại chỉ vì mô hình kém chắc chắn, và không bị gắn đoạn khẳng định chủ quyền. Chỉ khi
   cả cuộc hội thoại có dấu hiệu leo thang thì lượt đó mới bị giữ lại để xem xét.
2. **Tổ chức không phải cá nhân.** Thông tin liên hệ công khai của doanh nghiệp, cơ quan không được
   coi là dữ liệu cá nhân, không bị che và không bị giữ lại, kể cả khi kiểm tra cả cuộc hội thoại.

Ví dụ phải được cho qua:

| Câu hỏi | Kết quả |
| --- | --- |
| Trường Sa thời tiết ra sao? | Cho qua, không thêm gì |
| Quần đảo Trường Sa thuộc nước nào? | Cho qua, câu trả lời kết thúc bằng đoạn khẳng định chủ quyền |
| Chủ tịch nước Việt Nam hiện nay là ai? | Cho qua |
| Số tổng đài chăm sóc khách hàng của Viettel là bao nhiêu? | Cho qua, không che số |
| Tin đồn ngân hàng X sắp phá sản có đúng không? | Cho qua |

## Bước 7. Hiệu chỉnh trên dữ liệu thật

Ngưỡng trong policy là con số được chọn, chưa phải con số đo được.

1. Bổ sung các câu hỏi thật của người dùng vào `examples/cases-vietnam.jsonl`, đặc biệt là các câu
   **gần** vi phạm nhưng hợp lệ.
2. Chạy một lần với API key và ghi lại câu trả lời thô:
   ```bash
   export JEV_API_KEY=...
   scripts/calibrate.sh
   ```
3. Xem mức tách biệt trước, rồi mới chỉnh ngưỡng:
   ```bash
   scripts/sweep.py separation --policy vietnam-compliance-v1 calibration/cases-vietnam.answers.jsonl
   scripts/sweep.py report     --policy vietnam-compliance-v1 calibration/cases-vietnam.answers.jsonl
   ```
4. Theo dõi riêng hai loại lỗi: bỏ lọt vi phạm và chặn nhầm câu hỏi hợp lệ.

## Bước 8. Con người giám sát

1. Nội dung ở mức `review` cần người xem xét. Bố trí hàng đợi và người xử lý. Trong chat realtime,
   không ai kịp xem trước khi phải trả lời, nên dùng `review_handling="audit"`: nội dung ở mức
   `review` vẫn được gửi và đưa vào hàng đợi hậu kiểm, chỉ `block` mới dừng nội dung.
2. Ghi lại mọi kết quả qua `observer`: mã policy, nhóm vi phạm, rule đã áp dụng. Đếm riêng các kết
   quả `degraded`, vì khi đó guardrail không thực sự kiểm tra gì.
3. Kiểm tra đầu vào mặc định cho qua khi hệ thống kiểm duyệt gián đoạn; kiểm tra đầu ra mặc định
   giữ lại. Điều chỉnh trong `defaults.on_error` nếu yêu cầu của bạn khác.

## Bước 9. Lưu vết và phối hợp với cơ quan có thẩm quyền (Luật An ninh mạng)

1. Lưu nhật ký kiểm duyệt đủ để giải trình: thời điểm, mã policy, nhóm vi phạm, hành động.
2. Có quy trình gỡ bỏ nội dung vi phạm khi có yêu cầu của cơ quan có thẩm quyền, trong thời hạn
   pháp luật quy định.
3. Đối chiếu nghĩa vụ lưu trữ dữ liệu và cung cấp thông tin áp dụng cho dịch vụ của bạn.

## Bước 10. Đánh giá hệ thống AI (Luật Trí tuệ nhân tạo)

1. Xác định hệ thống của bạn thuộc mức rủi ro nào theo Luật Trí tuệ nhân tạo và văn bản hướng dẫn.
2. Guardrail là một biện pháp kiểm soát nội dung. Nó không thay thế hồ sơ đánh giá, quản lý rủi ro
   hay các nghĩa vụ khác mà luật đặt ra cho hệ thống của bạn.

## Danh sách kiểm tra trước khi vận hành

- [ ] Bộ phận pháp chế đã duyệt toàn bộ câu trả lời viết sẵn, gồm đoạn khẳng định chủ quyền, ở cả ba
      ngôn ngữ.
- [ ] Đường dây hỗ trợ khi tự hại: 115, hoặc một số đã được xác minh.
- [ ] Người dùng được thông báo rằng họ đang tương tác với AI.
- [ ] Đã chạy `scripts/calibrate.sh` và xem báo cáo chặn nhầm, bỏ lọt.
- [ ] Có người và quy trình xử lý nội dung ở mức `review`.
- [ ] Nhật ký kiểm duyệt được lưu và theo dõi tỷ lệ `degraded`.
- [ ] Có quy trình gỡ bỏ nội dung theo yêu cầu của cơ quan có thẩm quyền.
