下面给你一份可以直接拿去评审的技术方案草稿，主题是：

# 基于 Prompt 的下游不稳定响应归一化技术方案

## 1. 背景

当前系统需要调用下游服务获取处理结果，但下游接口存在以下问题：

1. 返回结构不稳定，不同版本字段命名不一致。
2. 有时返回标准 JSON，有时返回半结构化文本，甚至可能返回 HTML 错误页。
3. 成功、失败状态表达不统一，可能使用 `code`、`status`、`errno`、`msg`、`error` 等不同字段。
4. 业务关键字段位置不固定，可能出现在 `data`、`result`、`payload` 或更深层嵌套中。
5. 下游变更频繁，纯规则适配维护成本高。

为了保证上游调用方始终获得稳定、统一、可预测的响应结构，需要在 Go 服务中增加一层**响应归一化能力**。该能力采用“规则优先 + Prompt 兜底 + Schema 校验”的方案实现。

---

## 2. 建设目标

本方案目标如下：

* 对下游不稳定响应进行统一转换。
* 对外暴露稳定协议，屏蔽下游格式漂移。
* 优先使用规则适配，提高性能和确定性。
* 对未知格式或复杂文本场景引入 Prompt 归一化能力。
* 对模型输出执行强校验，确保最终结果可控。
* 为后续扩展多个下游、多个版本提供统一接入框架。

---

## 3. 适用范围

适用于以下场景：

* 第三方接口返回结构不稳定。
* 下游多版本并存，字段命名不统一。
* 返回内容可能为文本、JSON、HTML 混合格式。
* 需要快速适配频繁变化的外部服务。
* 上游要求固定格式返回。

不建议全量使用 Prompt 的场景：

* 极低延迟要求链路。
* 超高并发核心交易链路。
* 对字段可审计性和确定性要求极高的金融场景。
* 已知返回格式稳定且规则易维护的场景。

---

## 4. 总体设计

### 4.1 架构思路

整体采用三层归一化架构：

```text
Client
  -> Go API
      -> Downstream Client
      -> Raw Response
      -> Rule Adapter
           -> 命中则直接输出
           -> 未命中则进入 Prompt Normalizer
      -> Schema Validator
      -> Unified Response
  <- Client
```

### 4.2 核心策略

1. **规则优先**
   已知格式、主流格式优先走规则解析，保证低延迟和高确定性。

2. **Prompt 兜底**
   对规则无法识别或关键字段缺失的响应，交给大模型进行语义归一化。

3. **强校验输出**
   Prompt 输出结果必须通过 JSON Schema 校验，不合法则触发兜底策略。

4. **统一协议对外**
   无论下游如何变化，对上游始终返回统一结构。

---

## 5. 统一响应协议设计

### 5.1 内部标准结构

定义统一响应结构如下：

```json
{
  "success": true,
  "code": "OK",
  "message": "success",
  "data": {
    "id": "123",
    "name": "Alice",
    "phone": "13800000000",
    "status": "active"
  },
  "warnings": [],
  "raw_summary": "下游返回成功，提取到用户信息",
  "confidence": 0.98
}
```

### 5.2 字段说明

* `success`：统一成功标识。
* `code`：统一后的业务状态码，始终为字符串。
* `message`：统一后的结果描述。
* `data`：业务关键信息。
* `warnings`：字段缺失、低置信度、原始格式异常等告警信息。
* `raw_summary`：对原始下游响应的摘要。
* `confidence`：归一化结果置信度，范围 0 到 1。

### 5.3 Go 结构定义

```go
type UnifiedResponse struct {
    Success    bool         `json:"success"`
    Code       string       `json:"code"`
    Message    string       `json:"message"`
    Data       UnifiedData  `json:"data"`
    Warnings   []string     `json:"warnings"`
    RawSummary string       `json:"raw_summary"`
    Confidence float64      `json:"confidence"`
}

type UnifiedData struct {
    ID     *string `json:"id"`
    Name   *string `json:"name"`
    Phone  *string `json:"phone"`
    Status *string `json:"status"`
}
```

---

## 6. 系统模块设计

## 6.1 Downstream Client

负责请求下游服务，获取原始响应。

职责：

* 发起 HTTP/gRPC 请求。
* 设置请求超时。
* 记录原始状态码、响应头、响应体。
* 返回原始响应给归一化层。

接口示例：

```go
type DownstreamClient interface {
    Call(ctx context.Context, req any) (*RawResponse, error)
}

type RawResponse struct {
    HTTPStatus  int
    ContentType string
    Body        []byte
}
```

---

## 6.2 Rule Adapter

负责已知格式的快速适配。

职责：

* 处理高频、规则明确的返回结构。
* 根据版本、字段特征、content-type 做命中判断。
* 输出统一结构。

建议采用多适配器注册模式：

```go
type RuleAdapter interface {
    Match(raw *RawResponse) bool
    Normalize(raw *RawResponse) (*UnifiedResponse, error)
}
```

可支持：

* `AdapterV1`
* `AdapterV2`
* `TextAdapter`
* `LegacyAdapter`

---

## 6.3 Prompt Normalizer

负责对未知格式、复杂文本、字段混乱场景做语义归一化。

职责：

* 构造 Prompt。
* 调用大模型。
* 获取标准 JSON。
* 返回中间归一化结果。

接口示例：

```go
type PromptNormalizer interface {
    Normalize(ctx context.Context, input NormalizeInput) (*UnifiedResponse, error)
}

type NormalizeInput struct {
    HTTPStatus  int
    ContentType string
    RawResponse string
}
```

---

## 6.4 Schema Validator

负责校验规则适配或 Prompt 输出是否满足统一结构约束。

校验项：

* 是否为合法 JSON。
* 顶层字段是否完整。
* 字段类型是否匹配。
* `confidence` 是否在 0 到 1 范围内。
* `warnings` 是否为数组。
* `data` 是否为合法对象。

校验失败则返回标准失败结果，或触发重试。

---

## 6.5 Normalize Service

作为归一化总入口，串联整个流程。

伪流程如下：

```text
1. 调用下游
2. 进行响应预处理
3. 依次尝试 Rule Adapter
4. 若规则适配成功，进入 Schema Validator
5. 若规则适配失败，进入 Prompt Normalizer
6. 对 Prompt 输出执行 Schema Validator
7. 通过则返回
8. 失败则进入兜底输出
```

---

## 7. 处理流程设计

## 7.1 正常流程

```text
客户端请求
  -> Go 服务调用下游
  -> 获取原始响应
  -> Rule Adapter 匹配
      -> 成功：输出统一响应
      -> 失败：进入 Prompt
  -> Prompt 输出 JSON
  -> Schema 校验
  -> 返回统一响应
```

## 7.2 异常流程

### 场景 1：下游返回 HTML

* 规则解析失败。
* Prompt 识别为 HTML 错误页。
* 输出失败统一结构。
* warnings 标记 `downstream returned html`.

### 场景 2：下游返回字段缺失

* Prompt 仅抽取到部分字段。
* 缺失字段填 `null`。
* warnings 标记缺失字段。

### 场景 3：Prompt 输出不合法 JSON

* 自动重试一次。
* 再次失败则返回 `NORMALIZE_FAILED`。

---

## 8. Prompt 设计方案

## 8.1 Prompt 目标

Prompt 仅承担以下职责：

* 识别原始响应语义。
* 提取目标字段。
* 映射成功/失败状态。
* 输出统一 JSON。

不承担以下职责：

* 猜测不存在的数据。
* 自主做业务决策。
* 输出解释性文本。

---

## 8.2 System Prompt

```text
你是一个接口响应归一化引擎。

你的任务是把输入的下游接口响应转换成固定 JSON 结构。

要求：
1. 只能输出合法 JSON，不允许输出 markdown、解释、注释。
2. 不允许编造原始响应中不存在的信息。
3. 缺失字段填 null，并在 warnings 中说明。
4. 如果无法判断是否成功，则输出：
   - success = false
   - code = "UNKNOWN"
   - message = "unable to determine"
5. confidence 范围为 0 到 1。
6. 输出字段类型必须严格正确。
```

---

## 8.3 User Prompt 模板

```text
请将以下下游接口响应转换为标准格式。

目标 JSON 结构：
{
  "success": boolean,
  "code": string,
  "message": string,
  "data": {
    "id": string|null,
    "name": string|null,
    "phone": string|null,
    "status": string|null
  },
  "warnings": string[],
  "raw_summary": string,
  "confidence": number
}

字段提取规则：
- success 从 code/status/errno/msg/error 等字段综合判断
- code 统一转换为字符串
- message 优先从 message/msg/error/err_msg 提取
- id 优先从 data.id/result.id/payload.user_id/user.id 提取
- name 优先从 data.name/result.username/payload.user_name/user.name 提取
- phone 优先从 data.phone/result.mobile/payload.phone_number/user.phone 提取
- status 优先从 data.status/result.state/payload.status_desc 提取

输入参数：
content_type={{content_type}}
http_status={{http_status}}
raw_response={{raw_response}}
```

---

## 8.4 Prompt 调用条件

满足以下任一条件时进入 Prompt：

* JSON 解析失败。
* 未命中任何规则适配器。
* 关键字段缺失。
* content-type 与 body 内容不一致。
* 响应为半结构化文本。
* 响应中存在未知嵌套结构。

---

## 9. 输出校验与兜底策略

## 9.1 校验规则

校验内容包括：

* 顶层 JSON 合法。
* 所有必填字段存在。
* 字段类型正确。
* `confidence` 在合法范围内。
* `warnings` 为字符串数组。

## 9.2 重试策略

当 Prompt 输出不符合 Schema 时：

1. 重新发起一次严格模式调用。
2. 第二次 Prompt 附加提示：仅输出合法 JSON，不允许任何额外字符。
3. 如仍失败，直接返回标准失败结构。

## 9.3 最终兜底结构

```json
{
  "success": false,
  "code": "NORMALIZE_FAILED",
  "message": "failed to normalize downstream response",
  "data": {
    "id": null,
    "name": null,
    "phone": null,
    "status": null
  },
  "warnings": ["llm normalization failed"],
  "raw_summary": "raw downstream response could not be normalized",
  "confidence": 0
}
```

---

## 10. 非功能设计

## 10.1 性能要求

建议控制如下：

* Rule Adapter 处理耗时：1 到 5ms。
* Prompt 归一化耗时：按模型能力控制在 300ms 到 1500ms。
* 总体接口超时：建议 2s 到 5s。
* Prompt 不建议覆盖全部流量，应只作为兜底。

## 10.2 高可用设计

* Rule Adapter 为主路径，避免模型故障导致全链路不可用。
* Prompt 调用失败时自动降级为标准失败结构。
* 下游和模型调用均应设置独立超时。
* 模型调用支持熔断、限流、重试。

## 10.3 观测性

需记录以下日志和指标：

### 日志

* request_id
* downstream_status
* downstream_content_type
* raw_response_sample
* adapter_name
* prompt_used
* normalize_result
* normalize_error

### 指标

* 下游请求成功率
* 规则适配命中率
* Prompt 兜底比例
* Prompt 归一化成功率
* Schema 校验失败率
* 平均归一化耗时
* P95/P99 延迟

---

## 11. 安全与合规

需要注意：

1. 原始下游响应可能包含敏感信息，日志必须脱敏。
2. Prompt 输入长度需控制，避免将超长原始响应全量送入模型。
3. 对 HTML、异常字符串、乱码内容做清洗与截断。
4. 对模型输出严格校验，禁止未校验结果直接透传上游。
5. 若涉及用户隐私字段，应增加字段级脱敏或最小化传输策略。

---

## 12. 项目代码结构建议

```text
/internal
  /downstream
    client.go
    raw_response.go
  /normalize
    service.go
    preprocessor.go
    validator.go
  /adapter
    adapter.go
    v1.go
    v2.go
    text.go
  /prompt
    normalizer.go
    prompt_builder.go
  /model
    unified_response.go
  /handler
    api_handler.go
```

---

## 13. 关键接口示例

### 13.1 Normalize Service

```go
type NormalizeService struct {
    client      DownstreamClient
    adapters    []RuleAdapter
    prompt      PromptNormalizer
    validator   Validator
}
```

### 13.2 核心方法

```go
func (s *NormalizeService) Process(ctx context.Context, req any) (*UnifiedResponse, error) {
    raw, err := s.client.Call(ctx, req)
    if err != nil {
        return buildDownstreamError(err), nil
    }

    preprocessed := preprocess(raw)

    for _, adapter := range s.adapters {
        if adapter.Match(preprocessed) {
            result, err := adapter.Normalize(preprocessed)
            if err == nil && s.validator.Validate(result) == nil {
                return result, nil
            }
        }
    }

    result, err := s.prompt.Normalize(ctx, NormalizeInput{
        HTTPStatus:  preprocessed.HTTPStatus,
        ContentType: preprocessed.ContentType,
        RawResponse: string(preprocessed.Body),
    })
    if err != nil {
        return buildNormalizeFailed(err), nil
    }

    if err := s.validator.Validate(result); err != nil {
        return buildNormalizeFailed(err), nil
    }

    return result, nil
}
```

---

## 14. 风险分析

## 14.1 模型输出不稳定

风险：Prompt 输出字段错误、格式错误。
措施：Schema 校验 + 一次重试 + 统一失败兜底。

## 14.2 成本增加

风险：Prompt 调用比纯规则解析贵。
措施：仅在规则失效时触发 Prompt。

## 14.3 延迟增加

风险：大模型处理会拉长响应时间。
措施：设置严格超时，限制输入长度，控制兜底比例。

## 14.4 错误归一化

风险：模型误读业务字段。
措施：在 Prompt 中明确字段映射优先级，保留 warnings 和 confidence。

## 14.5 下游持续漂移

风险：Prompt 兜底比例持续上升。
措施：根据日志回放补充 Rule Adapter，形成闭环优化。

---

## 15. 演进路线

### 第一阶段

* 完成统一结构定义。
* 实现 Downstream Client。
* 实现 2 到 3 个主流 Rule Adapter。
* 建设基础 Validator。
* 接入 Prompt 兜底。

### 第二阶段

* 增加更多版本适配器。
* 引入指标监控与告警。
* 完善 Prompt 模板与失败分类。
* 增加原始响应回放能力。

### 第三阶段

* 建立自动发现新格式机制。
* 通过离线样本评估 Prompt 归一化准确率。
* 持续把高频未知格式沉淀成规则适配器。

---

## 16. 结论

本方案通过“**规则优先、Prompt 兜底、Schema 校验、统一协议对外**”的方式，解决下游响应不稳定带来的集成复杂性问题。

它的价值在于：

* 隔离外部系统不确定性。
* 保证上游接口稳定。
* 降低频繁维护适配规则的成本。
* 为多下游、多版本、多格式场景提供可扩展框架。

推荐作为当前场景的落地方案，并优先以“规则主路径 + Prompt 兜底”的模式上线，避免全量依赖模型带来的成本和时延问题。

如果你要，我下一步可以把这份方案继续补成更正式的版本，包括“时序图、流程图、异常码设计、接口示例、测试用例设计”。
