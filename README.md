# 洞穴微气候承载试验放行服务

本项目面向洞穴遗址保护监测员和独立环境复核员，管理一次候选区段从无人扰动基线冻结、低/中/高三级访客负荷采样、阈值判定、异常隔离与恢复验证，到独立复核及有限开放许可或拒绝归档的完整流程。

服务只提供版本化 HTTP JSON API，不依赖外部系统。每个写请求必须携带 `request_id`、`expected_revision` 和 `actor_id`。`expected_revision` 防止陈旧写入，`request_id` 与请求指纹保证重试得到稳定响应。试验数据、幂等索引和带前序摘要的审计链以原子 JSON 文件保存在本地数据目录中。

## 构建、运行和测试

需要 Go 1.22 或更高版本。

```text
go build ./...
go test ./...
go run ./cmd/server -addr=127.0.0.1:19081 -data-dir=./data
```

监听地址默认是 `127.0.0.1:19081`。`-addr` 可指定其他完整回环地址；未显式传入 `-addr` 时，也可以用纯数字 `PORT` 选择端口，服务会绑定 `127.0.0.1:<PORT>`。程序拒绝 `0.0.0.0`、空主机及其他非回环地址。

运行有界全流程自检：

```text
go run ./cmd/server -self-check -addr=127.0.0.1:19081
```

自检使用临时数据目录和真实回环 HTTP 请求，依次创建试验、提交三级负荷观测、触发异常暂停、验证连续恢复、完成独立复核、签发许可并校验证据包，随后主动关闭服务并退出。

## API 概览

所有写请求使用 `Content-Type: application/json`，请求体上限为 1 MiB，并拒绝未知字段。

- `POST /api/v1/trials`：创建候选区段；每台已校准传感器通过 `readings` 提交连续无人扰动序列。服务先校验单传感器稳定性，再按各序列首末时间的交集校验共同覆盖，并冻结末次读数陈旧度、跨传感器对齐偏差、逐台时间边界和决定性传感器。
- `POST /api/v1/trials/{trial_id}/observations`：按 `low`、`medium`、`high` 顺序提交人数严格递增、时长不递减且满足静置间隔的负荷阶段；`sampling_interval_seconds` 用于计算逐传感器应有点数、断采、边界覆盖和多传感器时间对齐。
- `POST /api/v1/trials/{trial_id}/assessment`：执行确定性阈值判定，冻结逐阶段、逐传感器结果，定位决定性采样点并计算绝对及百分比安全余量；同次判定按真实相邻采样时间执行正向梯形积分，返回各阶段传感器的温度、湿度和高于基线 CO₂ 暴露量、每分钟归一值及恶化趋势。
- `POST /api/v1/trials/{trial_id}/recovery`：提交 `measure_completed_at`、去重后的隔离措施和本次恢复批次；未达标批次也会增加修订并保留失败原因，连续窗口通过后才进入待复核。
- `POST /api/v1/trials/{trial_id}/reviews`：由未参与监测、隔离或恢复的人员批准或结构化拒绝；服务端自动对账阶段完整性、实际采样时点校准、判定输入摘要和恢复事实。
- `GET /api/v1/trials/{trial_id}`：读取聚合状态与全部业务事实。
- `GET /api/v1/trials/{trial_id}/timeline`：读取完整摘要链时间线。
- `GET /api/v1/trials/{trial_id}/evidence`：下载固定字段顺序的冻结证据包。
- `GET /api/v1/trials/{trial_id}/evidence/verification`：重算内容、审计链和终态摘要，并把冻结聚合推导出的建档、三级观测、判定、全部恢复尝试和唯一终态事件与时间线逐项进行数量、顺序、操作者及资源事实语义对账。
- `GET /healthz`：健康检查。

默认规则 `cave-clearance-rules/v2` 的停止阈值是温度增量 `1.5°C`、相对湿度增量 `6%` 和 CO₂ 峰值 `1200 ppm`。无人扰动基线默认每台传感器至少三个点、单台及共同覆盖跨度至少十分钟，末次读数距试验开始不超过 120 分钟，传感器末次读数对齐偏差不超过 60 秒；对应建档规则字段为 `baseline_min_span_minutes`、`max_baseline_staleness_minutes` 和 `max_baseline_alignment_seconds`。阶段间默认静置五分钟。恢复至少需要每台基线传感器三个连续、不恶化且满足更严格恢复阈值的采样点，连续跨度至少两分钟。调用方可以在建档时通过 `thresholds` 提交内部一致的区段规则。

许可和拒绝都是不可修改终态。许可中的最大同时人数和单次停留时长必须成对取自首次超限前的最高安全阶段，许可证据冻结依据阶段、安全余量和推导规则版本；没有安全阶段时只能拒绝。拒绝必须包含由 `code` 与 `detail` 组成的结构化原因，其证据包内容摘要和拒绝终态锚点在终态事务内一次冻结，重复下载保持 `generated_at` 与 `content_digest` 一致。
