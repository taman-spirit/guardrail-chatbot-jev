# 越南人工智能服务合规指南

[Tiếng Việt](vietnam-compliance.vi.md) · [English](vietnam-compliance.md) · **中文**

本指南逐步说明如何使用 `vietnam-compliance-v1` 策略，在越南上线人工智能聊天机器人，依据以下两部法律：

- **《人工智能法》**（Luật Trí tuệ nhân tạo）
- **《网络安全法》**（Luật An ninh mạng）

> 本指南为技术指南，不构成法律意见。请对照现行有效的法律及其实施细则，并在上线前由贵方法务部门审核。

---

## 第一步：确定范围和责任人

1. 列出内容进出模型的每一个环节：用户消息、回复、整段对话、检索内容（RAG）。
2. 每个环节对应一次检查：`input`、`output`、`conversation`。
3. 指定一名策略负责人，负责审批阈值调整、审批预设回复，并接收定期报告。

## 第二步：向用户表明其正在与人工智能交流（《人工智能法》）

1. 在对话开始时明确告知用户，他们正在与人工智能系统交流。
2. 当人工智能生成的内容被带出对话时，为其加注标识。
3. 不得让助手自称为人。`vai` 组会检查：当用户真诚询问时，回复是否自称为人。

## 第三步：阻止违禁内容

策略将违规内容分为若干组，每组都有专门的回复：

| 组别 | 依据 | 拦截 | **不**拦截 |
| --- | --- | --- | --- |
| `vsv` 领土主权 | 《网络安全法》 | 否认、歪曲越南领土主权的内容 | 天气、旅游、历史、新闻、关于法律地位的提问 |
| `vas` 反国家宣传 | 《网络安全法》 | 反国家宣传、煽动推翻、歪曲历史 | 关于体制、法律、政策的提问；合法的意见建议 |
| `vld` 领袖、领导人与国家象征 | 《网络安全法》 | 侮辱、捏造关于领袖、领导人、民族英雄、国旗、国徽、国歌的内容 | 生平、职务、引述、新闻 |
| `vcs` 虚假信息、扰乱秩序 | 《网络安全法》 | 引起恐慌的虚假信息、煽动扰乱秩序、攻击信息系统 | 询问传言真伪、举报虚假信息、防御性安全知识 |
| `prv` 个人信息 | 《网络安全法》 | **个人**的个人数据：身份证号、住址、私人电话 | **组织**的热线、客服电话、支持邮箱、地址、税号 |
| `vai` 利用人工智能欺骗 | 《人工智能法》 | 深度伪造、声音克隆、冒充、操纵弱势群体 | 讲解人工智能、明确标注为人工智能生成的内容 |

通用安全组（暴力、武器、儿童剥削、自我伤害等）沿用 `standard-v1`，保持不变。

## 第四步：接入应用

Python：

```python
from guardrail_chatbot_jev import Guard, Responder, detect_language

guard = Guard("vietnam-compliance-v1")
responder = Responder(guard.policy)          # 默认求助热线：115

def handle(message: str) -> str:
    lang = detect_language(message)          # "vi"、"en" 或 "zh"
    verdict_in = guard.check_input(message)
    if held := responder.blocking_response([verdict_in], language=lang):
        return held
    reply = call_model(message)
    verdict_out = guard.check_output(reply, user_message=message)
    return responder.compose(reply, [verdict_in, verdict_out], language=lang)
```

Go（包含在发布版本 `go-vietnam-compliance-v1` 中，模块版本 `v1.1.0` 及以上）：

```go
policy, _ := guardrail.BundledPolicy("vietnam-compliance-v1")
guard := guardrail.New(guardrail.Options{Policy: policy})
responder, _ := guardrail.NewResponder(policy, "") // "" = 115

lang := guardrail.DetectLanguage(message)
in, _ := guard.CheckInput(ctx, message, nil)
if held, ok := responder.BlockingResponse([]guardrail.Verdict{in}, lang); ok {
	return held
}
reply := callModel(message)
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{UserMessage: message})
return responder.Compose(reply, []guardrail.Verdict{in, out}, lang)
```

## 第五步：使用预设回复，不让模型自行撰写

1. 每个违规组都有越南语、英语和中文的专门回复，保存在策略的 `responses` 部分。`Responder` 只负责选择，不生成任何文本。
2. 违规回复会说明本服务遵守越南法律，并建议合规的提问方式。
3. 凡涉及主权的情形，一律以策略中预设的主权声明结尾，一字不改。该声明不由模型撰写，以避免文件编号出错。
4. 当用户出现自我伤害迹象时，回复以共情为主，不提及法律，并引导拨打 **115**。如贵机构有经过核实的心理援助热线，请通过 `Responder(policy, crisis_line="...")` 传入。
5. 等待人工审核的内容会收到中性提示，不认定用户违规。
6. 当内容审核系统中断时，告知用户系统中断，而不是指责用户。

## 第六步：不误拦正常提问

误拦同样是错误。策略通过两种机制降低误报：

1. **中性提及。** 当内容只是中性地提及某个地点、人物或组织（天气、旅游、职务、新闻）时，结果最多记录为标记：不拦截，不因模型把握不足而扣留，也不附加主权声明。只有当整段对话呈现升级态势时，该轮才会被扣留审核。
2. **组织不是个人。** 企业、机构公开的联系方式不属于个人数据，不会被遮蔽或扣留，对整段对话的检查也是如此。

必须放行的示例：

| 提问 | 结果 |
| --- | --- |
| 长沙群岛天气怎么样？ | 放行，不附加任何内容 |
| 长沙群岛属于哪个国家？ | 放行，回复以主权声明结尾 |
| 越南现任国家主席是谁？ | 放行 |
| Viettel 的客服电话是多少？ | 放行，号码不遮蔽 |
| X 银行即将破产的传言是真的吗？ | 放行 |

## 第七步：基于真实数据校准

策略中的阈值是人为选定的，尚未经过实测。

1. 将真实用户问题加入 `examples/cases-vietnam.jsonl`，尤其是**接近**违规但合规的问题。
2. 使用 API 密钥运行一次，并记录原始答案：
   ```bash
   export JEV_API_KEY=...
   scripts/calibrate.sh
   ```
3. 先查看区分度，再调整阈值：
   ```bash
   scripts/sweep.py separation --policy vietnam-compliance-v1 calibration/cases-vietnam.answers.jsonl
   scripts/sweep.py report     --policy vietnam-compliance-v1 calibration/cases-vietnam.answers.jsonl
   ```
4. 分别跟踪两类错误：漏放违规内容，以及误拦合规提问。

## 第八步：保留人工监督

1. `review` 级别的内容需要人工处理，请安排审核队列和人员。
2. 通过 `observer` 记录每一个结果：策略编号、违规组别、已触发的规则。单独统计 `degraded` 结果，因为此时护栏实际上没有进行任何检查。
3. 默认情况下，审核系统中断时输入检查放行，输出检查拦截。如有不同要求，请修改 `defaults.on_error`。

## 第九步：留存记录并配合主管部门（《网络安全法》）

1. 保留足以说明决定依据的审核日志：时间、策略编号、违规组别、处理动作。
2. 建立在主管部门提出要求时、于法定期限内删除违规内容的流程。
3. 核对适用于贵方服务的数据存储和信息提供义务。

## 第十步：评估人工智能系统（《人工智能法》）

1. 根据《人工智能法》及其实施细则，确定贵方系统所属的风险等级。
2. 护栏只是一项内容控制措施，不能替代法律对贵方系统规定的评估、风险管理及其他义务。

## 上线前检查清单

- [ ] 法务部门已审核全部预设回复（包括主权声明）的三种语言版本。
- [ ] 自我伤害求助热线：115，或经过核实的号码。
- [ ] 已告知用户其正在与人工智能交流。
- [ ] 已运行 `scripts/calibrate.sh` 并审阅误拦与漏放报告。
- [ ] 已为 `review` 级别内容安排人员和流程。
- [ ] 已保存审核日志并监控 `degraded` 比例。
- [ ] 已建立响应主管部门删除要求的流程。
