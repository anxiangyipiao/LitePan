# drivers/template — 新驱动脚手架

复制本目录为 `drivers/<your_driver>/`，按清单改代码。**不要**在 `drivers/all.go` 里 import template。

## 这目录是干什么的（白话）

接一个新网盘 = 写一套「怎么跟对方服务器说话」的代码。  
template 就是**样板房**：文件怎么分、HTTP 怎么发、Token 过期怎么办，都给你摆好，复制后只改平台特有的部分。

## 源码文件（网盘驱动统一集合）

每个网盘驱动目录**只允许**下列 `.go` 源码文件：

| 文件 | 干什么 |
|------|--------|
| `config.go` | 账号表单字段（Addition）、`flexString` 等配置解析 |
| `driver.go` | 注册、Init/Drop/Ping、**ListFiles / GetFileInfo**、列表分页 helper、能力断言 |
| `transport.go` | API 常量、延迟、`apiCall`、错误码映射 |
| `models.go` | 平台 JSON → `domain.FileItem` / 下载结构体 |
| `auth.go` | Token 刷新、认证失败分类 |
| `ops.go` | **ResolveDownload**、删/移/复制/重命名/建目录、下载探针等写操作辅助 |
| `upload.go` | 本地上传、秒传/哈希复用、OSS 分片等上传实现 |
| `qrlogin.go` | **仅**扫码/短信登录类驱动需要（如夸克） |

**禁止**再拆 `list.go`、`transfer.go`、`upload_oss.go` 等额外源码文件；逻辑并入上表对应文件。

`drivers/` 下不写单测，联调在真机环境验证。

`localfs`、`mock` 等非网盘驱动可更简，但 Token/ Cookie 网盘应对齐上表。

## 和 `internal/httpx` 的关系

httpx 是**所有驱动共用的 HTTP 小工具**，不是替换 Go 标准库，底层仍是 `net/http`：

| 函数 | 用途 |
|------|------|
| `NewClient` | 带超时/代理的 HTTP 客户端 |
| `NewJSONRequest` | 拼 JSON 请求 |
| `SetHeaders` | 批量设请求头 |
| `DoJSON` | 上面三步 + 发请求 + 读 body（transport 最常用） |
| `Execute` | 发请求 + 限长读 body |
| `ParseDataEnvelope` | 解析 `{code, message, data}`，把 `data` 解进你的结构体 |
| `Truncate` | 截断错误日志里的响应体 |

**Cookie 型驱动（如夸克）** 不用 `{code,data}` 外壳，在 `transport.go` 里自己解析，但仍用 `DoJSON` / `Execute`。

## 接入清单（复制后逐项打勾）

1. 读设计方案 §3，列出本驱动要实现的小接口
2. 读 LitePan Python 原驱动：认证、删除/下载/上传约束
3. 核对目标网盘接口：签名、分页、Range
4. 改 `Addition` 字段与 form tag
5. 改 `config.go` 里 `Name` / `DisplayName` / `CardLogo`
6. 实现 `models.go`：API 响应 → `domain`
7. 改 `transport.go` 里平台域名、请求头、错误码映射（`mapAPIError`）
8. Cookie 驱动改 `auth.go`；Token 驱动保留刷新路径
9. 在 `ops.go` 实现 **ResolveDownload** / 删改移等
10. 在 `upload.go` 实现上传与秒传（若有）
11. `init()` 保留；在 `drivers/all.go` 加 `_ "litepan/drivers/<name>"`
12. 真机联调列表、下载、上传、删除、移动、重命名、秒传等实际链路；本地至少过编译和全量 Go 测试

## 参考实现

| 类型 | 看哪个 |
|------|--------|
| Token + `{code,data}` | `drivers/123_Open/` |
| Cookie + 自定义外壳 | `drivers/Quark/`（含 `qrlogin.go`） |
| 列表分页 + 下载探针 | `drivers/115_Open/`、`drivers/Guangya/` |
| 本地 / 极简 | `drivers/LocalFs/` |

## 验收

```bash
go test ./...
go test -race ./...
go build ./drivers/<name>/...
```
