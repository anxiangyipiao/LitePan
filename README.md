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

> **一句话简介**：LitePan 是轻量、多网盘聚合的个人云盘中枢——把分散在各家的文件聚到一个界面，配上离线下载 / 跨盘秒传 / STRM 直连播放 / 刮削 / 自动联动 / 挂载共享，一站打通「找资源 → 落盘 → 整理 → 播」。

## ▎ 为什么选 LitePan

| 痛点 | LitePan 怎么做 |
|---|---|
| 网盘太多、来回切换 | 多账号聚合，一屏看完；支持 115 / 123 / 沃云盘 / 夸克 / 天翼 / 百度 / OneDrive / WebDAV / 本地盘… |
| 找资源、落盘麻烦 | 内置 **nyaa 磁力搜索 + 一键离线到网盘 + 一键推 qBittorrent**，磁力直达 |
| 大文件跨盘搬运慢 | **跨盘秒传**优先，秒传失败自动走上传；支持断点、并发 |
| 影视想用 Emby / Jellyfin / 飞牛看 | **STRM 直连播放** + **刮削（TMDB / MetaTube）**写 nfo / 海报，海报墙可追更 |
| 目录乱、命名不规范 | **目录整理（TMDB 识别）**：预览后再执行，可回滚；散落电影按文件独立建夹 |
| 每步都要手动点 | **自动联动（Automation）**把整理 / STRM / 刮削 / 刷库串起来；支持 **RSS → 磁力 → qB** 等玩法 |
| 想当本地盘 / 被播放器直接读 | **WebDAV + FUSE 本地挂载**（含读缓存），302 直链与代理下载可选 |

## ▎ 本分支亮点（fork 差异）

本仓库是 [LitePan](https://github.com/Ponphil/LitePan) 的分支，保留上游全部能力，并增量：

| 差异 | 说明 |
|------|------|
| 🆕 **联通沃云盘驱动** | 新增中国联通「沃云盘」（pan.wo.cn）：个人 / 家庭空间均可挂载，refresh_token 直连换新、自动续期，302 直链与代理下载 |
| ⬆️ **天翼云盘增强** | 家庭空间新增「家庭空间 ID」可手动指定，多家庭空间精确挂载；留空回退上游自动选择 |
| ⬆️ **云盘别名** | 聚合不同网盘文件夹到同一个虚拟目录 |
| ⬆️ **nyaa 磁力搜索** | 首页内置磁力搜索（需代理），搜到即 **复制 / 一键离线到网盘 / 一键推 qB** 三选一 |
| ⬆️ **STRM 刮削增强** | 新增 **MetaTube** 数据源，TMDB 之外的更全覆盖；需自建 MetaTube 服务 |
| 🎨 **移动端重构** | 前端全新布局：存储盘顶部横向卡片、双栏（收藏夹 + 文件区）、面包屑贴近文件区，手持更顺手 |

### 前端重构亮点

- **存储盘横向卡片条**：全部网盘一目了然，一键切换；
- **双栏布局**：左侧收藏夹、右侧文件区，视觉更聚焦；
- **路径导航贴近文件**：面包屑紧贴列表，点击区域更大；

![前端预览](docs/pictures/preview.png)

### 📦 fnOS `.fpk` 安装包

由 [LitePan-fpk](https://github.com/anxiangyipiao/LitePan-fpk) 自动构建，`x86` / `arm64` 双架构，下载后在 fnOS 中安装，默认账号密码均为 `admin`。

<br>

## ▎ 核心功能一览

<table>
  <tr>
    <td width="50%" valign="top" align="center">
      <h3>多网盘聚合</h3>
      <p align="left">多账号统一管理，一屏看完；支持 10+ 主流网盘与本地/WebDAV。</p>
      <img src="docs/pictures/feature-browser.png" alt="多网盘聚合" height="220">
    </td>
    <td width="50%" valign="top" align="center">
      <h3>跨盘秒传</h3>
      <p align="left">能秒传就秒传，否则自动上传；支持大文件、断点与并发。</p>
      <img src="docs/pictures/feature-crosstransfer.png" alt="跨盘秒传" height="220">
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top" align="center">
      <h3>磁力搜索 · 离线下载 · 推 qB</h3>
      <p align="left">nyaa 磁力一键 <b>离线到网盘</b>（仅支持的盘）或 <b>推本地 qBittorrent</b>，也支持复制磁力。</p>
      <img src="docs/pictures/feature-browser.png" alt="磁力与离线" height="220">
    </td>
    <td width="50%" valign="top" align="center">
      <h3>STRM 直连播放</h3>
      <p align="left">生成 <code>.strm</code> 直连播放，对接 Emby / Jellyfin / 飞牛影视。</p>
      <img src="docs/pictures/feature-strm.png" alt="STRM 直连播放" height="220">
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top" align="center">
      <h3>STRM 刮削</h3>
      <p align="left">TMDB / MetaTube 双源，写 nfo / 海报，海报墙可追更。</p>
      <img src="docs/pictures/feature-strm-scrape.png" alt="STRM 刮削" height="220">
    </td>
    <td width="50%" valign="top" align="center">
      <h3>目录整理（TMDB 识别）</h3>
      <p align="left">智能识别影视，预览后再归档；散落电影按文件独立建夹。</p>
      <img src="docs/pictures/feature-organize.png" alt="目录整理" height="220">
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top" align="center">
      <h3>自动联动</h3>
      <p align="left">把整理、STRM、刮削、刷库串起来；支持 Webhook / 定时触发。</p>
      <img src="docs/pictures/feature-automation.png" alt="自动联动" height="220">
    </td>
    <td width="50%" valign="top" align="center">
      <h3>挂载与共享</h3>
      <p align="left">WebDAV + FUSE 本地挂载（含读缓存），302 直链 / 代理下载可选。</p>
      <img src="docs/pictures/feature-strm.png" alt="挂载与共享" height="220">
    </td>
  </tr>
</table>

> 更多：缓存策略 / 命名对齐 / 家庭空间 / 别名聚合 / 离线任务管理，见站内「系统设置」与「辅助工具」。

<br>

## ▎ 支持的网盘

`115_Open` · `123_Open` · `沃云盘 (wopan)` · `光鸭 (Guangya)` · `139Cloud / 天翼` · `Baidu_Open` · `Quark` · `OneDrive` · `WebDAV` · `LocalFs` · `alias（别名聚合）`

> 离线下载：仅支持具备离线能力的驱动（如 115、光鸭等，`magnet` 需 `url_schemes` 含 `magnet`）；不支持的账号在“离线到网盘”弹窗中自动隐藏。

<br>

## ▎ Docker 部署

### 环境要求

- Docker ≥ 20.10
- Docker Compose ≥ 2.0

### 方式一：docker-compose（推荐）

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

<br>

## ▎ 关键配置提示

- **磁力搜索代理**：`系统设置 → 其他设置` 填 `magnet_search_proxy_*`，nyaa 站点不可直连时必填；
- **qBittorrent 一键推送**：`系统设置 → 其他设置 → qBittorrent` 填 `qb_url`（如 `http://192.168.1.10:8080`，Docker 内勿填 `127.0.0.1`，用 `host.docker.internal` 或宿主机 IP）+ 账号，卡片内可 **测试连接**；
- **MetaTube 刮削**：需自建 MetaTube 服务后在 `STRM 刮削` 中选择数据源为 MetaTube 并填服务地址；
- **离线下载**：在磁力结果点 **离线到网盘**，仅展示支持 `magnet` 的账号（115/光鸭等）。

<br>

## ▎ 支持

<table>
  <tr>
    <td width="50%" valign="top">
      <h3>支持 LitePan</h3>
      <p>觉得有用请点右上角 <strong>Star</strong>，也欢迎自愿赞赏。</p>
      <img src="docs/pictures/wechat-tip.png" alt="微信赞赏" width="260">
    </td>
  </tr>
</table>

## ▎ 反馈

外部贡献致谢见 [ACKNOWLEDGEMENTS.md](./ACKNOWLEDGEMENTS.md)。

---

## ▎ 许可

[PolyForm Noncommercial 1.0.0](./LICENSE) — 个人学习与非商业使用，**禁止商用**。  
第三方依赖见 [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)。请遵守各网盘服务条款与当地法规。

[docker-pulls-shield]: https://img.shields.io/docker/pulls/ponphil/litepan?logo=docker&logoColor=white&style=flat-square
[version-shield]: https://img.shields.io/badge/Version-v0.5.0--Beta-6C63FF?style=flat-square
[license-shield]: https://img.shields.io/badge/License-PolyForm%20NC-red?style=flat-square
[docker-url]: https://hub.docker.com/r/ponphil/litepan
[license-url]: ./LICENSE
