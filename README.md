<a name="readme-top"></a>

<div align="center">

<img src="docs/pictures/banner.png" alt="LitePan" width="100%">

<br>

<a href="https://www.litepan.top"><img src="https://img.shields.io/badge/官网文档-www.litepan.top-6C63FF?style=for-the-badge&labelColor=1B1B2F" alt="官网文档"></a>
&nbsp;
<a href="https://space.bilibili.com/1501989416"><img src="https://img.shields.io/badge/Bilibili-交流与演示-00A1D6?style=for-the-badge&logo=bilibili&logoColor=white&labelColor=1B1B2F" alt="Bilibili"></a>
&nbsp;
<a href="https://hub.docker.com/r/ponphil/litepan"><img src="https://img.shields.io/badge/Docker-ponphil%2Flitepan-2496ED?style=for-the-badge&logo=docker&logoColor=white&labelColor=1B1B2F" alt="Docker"></a>


[![docker-pulls][docker-pulls-shield]][docker-url]
[![version][version-shield]][docker-url]
[![license][license-shield]][license-url]

</div>

## LitePan — 轻量多网盘聚合的个人云盘中枢

> **一句话简介**：把分散在各家网盘的文件聚到一个界面，配上磁力搜索 / 离线下载 / 跨盘秒传 / STRM 直连播放 / 刮削 / 自动联动 / 挂载共享，一站打通「找资源 → 落盘 → 整理 → 播」的全流程。

---

## 为什么选 LitePan

| 痛点 | LitePan 怎么做 |
|---|---|
| 网盘太多、来回切换 | 多账号聚合，一屏看完；支持 10+ 主流网盘与本地/WebDAV |
| 找资源、落盘麻烦 | 内置 **9 大磁力搜索源 + 一键离线到网盘 + 一键推 qBittorrent**，磁力直达 |
| 大文件跨盘搬运慢 | **跨盘秒传**优先，秒传失败自动走上传；支持断点、并发 |
| 影视想用 Emby / Jellyfin / 飞牛看 | **STRM 直连播放** + **刮削（TMDB / MetaTube）**写 nfo / 海报，海报墙可追更 |
| 目录乱、命名不规范 | **目录整理（TMDB 识别）**：预览后再执行，可回滚；散落电影按文件独立建夹 |
| 每步都要手动点 | **自动联动（Automation）**把整理 / STRM / 刮削 / 刷库串起来；支持 **RSS → 磁力 → qB** 等玩法 |
| 想当本地盘 / 被播放器直接读 | **WebDAV + FUSE 本地挂载**（含读缓存），302 直链与代理下载可选 |

---

## 核心功能详解

### 1. 多网盘聚合

LitePan 是一个真正的**多网盘聚合平台**。不管你有 3 个网盘还是 10 个网盘，所有文件都能在一个界面里统一管理。

**支持的网盘驱动：**
- **115 网盘**（115_Open）
- **123 网盘**（123_Open）
- **联通沃云盘**（wopan）—— 个人/家庭空间均可挂载，refresh_token 自动续期
- **光鸭**（Guangya）
- **天翼云盘**（139Cloud）—— 支持家庭空间 ID 手动指定，多家庭空间精确挂载
- **百度网盘**（Baidu_Open）
- **夸克网盘**（Quark）
- **OneDrive**
- **WebDAV** —— 任何支持 WebDAV 的服务都能接入
- **本地文件系统**（LocalFs）
- **别名聚合**（alias）—— 把不同网盘的文件夹映射到同一个虚拟目录

**界面亮点：**
- 存储盘顶部**横向卡片条**，全部网盘一目了然，一键切换
- **双栏布局**：左侧收藏夹、右侧文件区，视觉更聚焦
- **路径导航贴近文件**：面包屑紧贴列表，点击区域更大
- 移动端响应式适配，手持设备体验流畅

---

### 2. 磁力搜索 · 离线下载 · 推 qBittorrent

LitePan 内置了 **9 大磁力搜索源**，覆盖主流磁力资源站点：

| 搜索源 | 域名 | 特点 |
|---|---|---|
| **sukebei** | sukebei.nyaa.si | 老牌番号站，单次请求直出 magnet |
| **cltt2** | cltt2.shop | 磁力天堂，JSON API + DES 加密，单次请求 |
| **seedhub** | seedhub.cc | 影视资源分享站，详情页 base64 解码 |
| **sobt** | sobt10.vip | 种子搜索，POST + cookie session |
| **btfox** | btfox20.top | BT 狐狸，GET 搜索 + base64 编码 |
| **btkitty** | btkitty0.com | BT 猫咪，POST + base64 解码 |
| **zzb** | zzb10.vip | 种子吧，GET 搜索 + HTML 解析 |
| **clm64** | clm64.top | 磁力猫，GET + base64 + cookie session |
| **cilibao** | 磁力宝 | GET 搜索，纯静态 HTML |

**搜索到资源后，一键操作三选一：**
- **复制磁力链** —— 直接复制到剪贴板
- **离线到网盘** —— 推送到支持离线下载的网盘（115 / 光鸭等）
- **推送到 qBittorrent** —— 本地下载，配合 STRM 直连播放

还支持**磁力收藏**，常用资源一键收藏，下次直接用。

---

### 3. 跨盘秒传

想把文件从 115 搬到夸克？传统方式要下载再上传，大文件等半天。

LitePan 的**跨盘秒传**功能：
- **优先走秒传通道** —— 如果目标盘已有相同文件，秒级完成
- **秒传失败自动降级** —— 自动切换为上传模式
- **支持断点续传** —— 大文件传输中断后可继续
- **支持并发** —— 多文件同时传输，充分利用带宽
- **进度可视化** —— 实时查看传输状态

---

### 4. STRM 直连播放 + 刮削

这是影视玩家最看重的功能，LitePan 提供完整的影视管理方案：

**STRM 直连播放：**
- 把网盘文件生成 `.strm` 文件
- 对接 **Emby / Jellyfin / 飞牛影视** 等媒体服务器
- **不下载就能看** —— 播放器通过 STRM 直连网盘播放，节省本地存储
- 支持批量生成，自动匹配目录结构

**刮削（TMDB / MetaTube 双数据源）：**
- **TMDB** —— 全球最大的影视数据库，覆盖中英文片名、演员、评分、海报
- **MetaTube** —— 自建刮削服务，补充 TMDB 未覆盖的内容
- 自动写入 **nfo 元数据** —— 播放器打开就能识别
- 自动下载**海报 / 背景图** —— 海报墙完整呈现
- **追更支持** —— 剧集更新后自动重新刮削

---

### 5. 目录整理（TMDB 识别）

下载的文件命名乱七八糟？LitePan 帮你自动整理：

- **TMDB 智能识别** —— 通过文件名自动识别影视内容
- **预览后再执行** —— 先看整理方案，满意再执行，不满意可以回滚
- **标准目录结构** —— 按「年份 / 片名 / 季集」自动归档
- **散落电影处理** —— 单独的电影文件自动创建文件夹并移入
- **命名规范化** —— 统一文件名格式，告别混乱

---

### 6. 自动联动（Automation）

真正的「自动化流水线」—— 把多个操作串成一条链：

**内置操作节点：**
- 目录整理
- STRM 生成
- 刮削（TMDB / MetaTube）
- Emby / Jellyfin 刷库
- 通知推送

**触发方式：**
- **手动触发** —— 点击运行
- **定时触发** —— Cron 定时执行
- **Webhook 触发** —— 外部系统调用触发

**典型玩法：**
- 下载完 → 自动整理 → 自动生成 STRM → 自动刮削 → 自动刷 Emby 库
- RSS 订阅 → 匹配关键词 → 自动离线到网盘 → 自动整理

---

### 7. 挂载与共享

LitePan 内置 WebDAV 服务和 FUSE 挂载，让网盘变成「本地盘」：

**WebDAV 服务：**
- 内置 WebDAV 服务器，任何支持 WebDAV 的播放器都能直接读取
- 支持 WebDAV 客户端（Windows / macOS / Linux）挂载

**FUSE 本地挂载：**
- 把网盘映射成一个本地磁盘
- **读取带缓存** —— 热门文件缓存到本地，体验和本地硬盘一样
- 支持多挂载点同时挂载

**下载模式：**
- **302 直链** —— 播放器直接从网盘下载，不经过 LitePan
- **代理下载** —— 流量经过 LitePan，支持限速和日志

---

### 8. 其他实用功能

- **RSS 订阅** —— 自动抓取动漫/影视 RSS，按关键词/集数/画质匹配，自动推送离线下载
- **qBittorrent 管理** —— 一键推送磁力到 qB，实时查看下载进度
- **缓存策略** —— 灵活的缓存配置，按需清理
- **系统日志** —— 完整的操作日志，问题排查一目了然
- **备份管理** —— 定时备份数据库和配置
- **通知系统** —— 任务完成/失败自动通知
- **API Keys** —— 支持 API 密钥管理，方便第三方集成

---

## 部署方式

### 方式一：Docker Compose（推荐）

```bash
git clone https://github.com/anxiangyipiao/LitePan.git
cd LitePan
docker compose up -d --build
```

访问 `http://你的IP:5211`，默认账号密码均为 `admin`。

```bash
docker compose logs -f          # 看日志
docker compose down             # 停止
docker compose up -d --build    # 代码更新后重建
```

### 方式二：飞牛 NAS（fnOS）

```bash
git clone https://github.com/anxiangyipiao/LitePan.git
cd LitePan
docker compose -f docker-compose.fnos.yml up -d --build
```

> fnOS 使用 `network_mode: "host"`，无需映射端口，直接访问 `http://你的IP:5211`。

也可以使用 `.fpk` 安装包（由 [LitePan-fpk](https://github.com/anxiangyipiao/LitePan-fpk) 自动构建），`x86` / `arm64` 双架构，下载后在 fnOS 中安装即可。

### 方式三：手动构建

```bash
docker build -t litepan .
docker run -d \
  --name litepan \
  -p 5211:5211 \
  -v ./data:/app/data \
  -v ./strm:/app/strm \
  -v ./mounts:/app/mounts:shared \
  --device /dev/fuse:/dev/fuse \
  --privileged \
  litepan
```

### 数据目录

| 容器路径 | 说明 | 建议 |
|---------|------|------|
| `/app/data` | 数据库、配置、日志 | 必须持久化 |
| `/app/strm` | STRM 输出目录 | 必须持久化 |
| `/app/mounts` | FUSE 挂载点 | 需 `:shared` |

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LITEPAN_DATA_DIR` | `/app/data` | 数据目录 |
| `LITEPAN_STRM_DIR` | `/app/strm` | STRM 输出目录 |
| `LITEPAN_LISTEN` | `:5211` | 监听地址 |
| `LITEPAN_LOG_LEVEL` | `info` | 日志级别（debug/info/warn/error） |
| `TZ` | `Asia/Shanghai` | 时区 |

---

## 关键配置提示

- **磁力搜索代理**：`系统设置 → 其他设置` 填 `magnet_search_proxy_*`，nyaa 站点不可直连时必填；
- **qBittorrent 一键推送**：`系统设置 → 其他设置 → qBittorrent` 填 `qb_url`（如 `http://192.168.1.10:8080`，Docker 内勿填 `127.0.0.1`，用 `host.docker.internal` 或宿主机 IP）+ 账号，卡片内可 **测试连接**；
- **MetaTube 刮削**：需自建 MetaTube 服务后在 `STRM 刮削` 中选择数据源为 MetaTube 并填服务地址；
- **离线下载**：在磁力结果点 **离线到网盘**，仅展示支持 `magnet` 的账号（115/光鸭等）；
- **沃云盘**：使用 refresh_token 认证，自动续期，个人/家庭空间均可挂载；
- **天翼云盘**：家庭空间支持手动指定家庭空间 ID，多家庭空间精确挂载。

---

## 技术栈

| 层级 | 技术 |
|------|------|
| **后端** | Go（单二进制，轻量无依赖） |
| **前端** | Vue 3 + TypeScript + Vite |
| **数据库** | SQLite（零配置，嵌入式） |
| **部署** | Docker / Docker Compose |
| **媒体刮削** | TMDB API / MetaTube |
| **文件协议** | WebDAV / FUSE / 302 直链 |

---

## 界面预览

![前端预览](docs/pictures/preview.png)

更多截图见 [docs/pictures/](./docs/pictures/) 目录。

---

## 支持

<table>
  <tr>
    <td width="50%" valign="top">
      <h3>支持 LitePan</h3>
      <p>觉得有用请点右上角 <strong>Star</strong>，也欢迎自愿赞赏。</p>
      <img src="docs/pictures/wechat-tip.png" alt="微信赞赏" width="260">
    </td>
  </tr>
</table>

## 反馈

外部贡献致谢见 [ACKNOWLEDGEMENTS.md](./ACKNOWLEDGEMENTS.md)。

---

## 许可

[PolyForm Noncommercial 1.0.0](./LICENSE) — 个人学习与非商业使用，**禁止商用**。  
第三方依赖见 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。请遵守各网盘服务条款与当地法规。

[docker-pulls-shield]: https://img.shields.io/docker/pulls/ponphil/litepan?logo=docker&logoColor=white&style=flat-square
[version-shield]: https://img.shields.io/badge/Version-v0.5.0--Beta-6C63FF?style=flat-square
[license-shield]: https://img.shields.io/badge/License-PolyForm%20NC-red?style=flat-square
[docker-url]: https://hub.docker.com/r/ponphil/litepan
[license-url]: ./LICENSE
