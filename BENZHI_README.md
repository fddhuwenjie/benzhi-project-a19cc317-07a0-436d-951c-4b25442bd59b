# BENZHI_README

## 项目说明
- 项目：benzhi-project-a19cc317-07a0-436d-951c-4b25442bd59b
- 项目用途：已实现洞穴遗址候选区段微气候承载试验放行服务，覆盖基线冻结、三级负荷采样、确定性阈值判定、异常隔离与连续恢复、独立复核、有限许可或拒绝终态、原子持久化、幂等重放、链式审计和确定性证据校验。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：cave-microclimate-clearance
- 项目介绍：面向洞穴遗址保护技术人员的微气候承载试验放行服务，以一次开放候选区段为唯一业务聚合，从基线冻结、受控扰动采样、阈值判定、恢复验证、独立复核推进到开放许可签发或拒绝归档，并保留可验证的决策证据。
- 项目概述：面向洞穴遗址保护技术人员的微气候承载试验放行服务，以一次开放候选区段为唯一业务聚合，从基线冻结、受控扰动采样、阈值判定、恢复验证、独立复核推进到开放许可签发或拒绝归档，并保留可验证的决策证据。
- 核心工作流：保护监测员登记一个开放候选区段并冻结无人扰动基线，提交分阶段访客负荷试验及温度、湿度、二氧化碳读数；系统执行完整性和阈值判定，异常时要求停止并完成恢复验证，随后由不同人员独立复核，最终签发带有效期和人数上限的开放许可或形成拒绝结论并冻结证据。
- 对外接口：仅提供版本化 HTTP JSON API；写请求携带 request_id 与 expected_revision，服务监听地址支持 -addr=127.0.0.1:<port>，默认 127.0.0.1:19081，并提供 -self-check 模式通过真实回环 HTTP 请求完成有界全流程后主动退出。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-a19cc317-07a0-436d-951c-4b25442bd59b-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-a19cc317-07a0-436d-951c-4b25442bd59b-arm64 linux/arm64

docker run -it benzhi-project-a19cc317-07a0-436d-951c-4b25442bd59b-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -self-check -addr=127.0.0.1:19081`
