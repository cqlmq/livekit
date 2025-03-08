<!--BEGIN_BANNER_IMAGE-->

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="/.github/banner_dark.png">
  <source media="(prefers-color-scheme: light)" srcset="/.github/banner_light.png">
  <img style="width:100%;" alt="The LiveKit icon, the name of the repository and some sample code in the background." src="https://raw.githubusercontent.com/livekit/livekit/main/.github/banner_light.png">
</picture>

<!--END_BANNER_IMAGE-->

# LiveKit: Real-time video, audio and data for developers (cqlmq)
# LiveKit: 为开发者提供的实时视频、音频和数据解决方案

## Git 常用命令速查
```bash
# 代码同步
git pull upstream master    # 从上游仓库拉取代码
git push origin master     # 推送到远程仓库
git fetch upstream         # 获取上游更新
git merge upstream/main    # 合并上游代码

# 分支操作
git checkout origin/master # 切换到远程主分支
git branch -a             # 查看所有分支
git status                # 查看当前状态
```

[LiveKit](https://livekit.io) is an open source project that provides scalable, multi-user conferencing based on WebRTC.

LiveKit 是一个基于 WebRTC 的开源项目，提供可扩展的多用户会议功能。

It's designed to provide everything you need to build real-time video audio data capabilities in your applications.

它的设计目标是为您的应用程序提供构建实时视频音频数据功能所需的一切。

LiveKit's server is written in Go, using the awesome [Pion WebRTC](https://github.com/pion/webrtc) implementation.

LiveKit 服务器使用 Go 语言编写，采用优秀的 Pion WebRTC 实现。

[![GitHub stars](https://img.shields.io/github/stars/livekit/livekit?style=social&label=Star&maxAge=2592000)](https://github.com/livekit/livekit/stargazers/)
[![Slack community](https://img.shields.io/endpoint?url=https%3A%2F%2Flivekit.io%2Fbadges%2Fslack)](https://livekit.io/join-slack)
[![Twitter Follow](https://img.shields.io/twitter/follow/livekit)](https://twitter.com/livekit)
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/livekit/livekit)](https://github.com/livekit/livekit/releases/latest)
[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/livekit/livekit/buildtest.yaml?branch=master)](https://github.com/livekit/livekit/actions/workflows/buildtest.yaml)
[![License](https://img.shields.io/github/license/livekit/livekit)](https://github.com/livekit/livekit/blob/master/LICENSE)

## Features
## 功能特点

-   Scalable, distributed WebRTC SFU (Selective Forwarding Unit)
    可扩展的分布式 WebRTC SFU（选择性转发单元）

-   Modern, full-featured client SDKs
    现代化、功能完整的客户端 SDK

-   Built for production, supports JWT authentication
    为生产环境构建，支持 JWT 认证

-   Robust networking and connectivity, UDP/TCP/TURN
    强大的网络和连接能力，支持 UDP/TCP/TURN

-   Easy to deploy: single binary, Docker or Kubernetes
    易于部署：单二进制文件、Docker 或 Kubernetes

-   Advanced features including:
    高级功能包括：
    -   [speaker detection](https://docs.livekit.io/home/client/tracks/subscribe/#speaker-detection)
        说话者检测
    -   [simulcast](https://docs.livekit.io/home/client/tracks/publish/#video-simulcast)
        多流传输
    -   [end-to-end optimizations](https://blog.livekit.io/livekit-one-dot-zero/)
        端到端优化
    -   [selective subscription](https://docs.livekit.io/home/client/tracks/subscribe/#selective-subscription)
        选择性订阅
    -   [moderation APIs](https://docs.livekit.io/home/server/managing-participants/)
        管理 API
    -   end-to-end encryption
        端到端加密
    -   SVC codecs (VP9, AV1)
        可扩展视频编码
    -   [webhooks](https://docs.livekit.io/home/server/webhooks/)
        网络钩子
    -   [distributed and multi-region](https://docs.livekit.io/home/self-hosting/distributed/)
        分布式和多区域部署

## Documentation & Guides
## 文档与指南

https://docs.livekit.io

## Live Demos
## 在线演示

-   [LiveKit Meet](https://meet.livekit.io) ([source](https://github.com/livekit-examples/meet))
    LiveKit 会议演示 ([源代码](https://github.com/livekit-examples/meet))

-   [Spatial Audio](https://spatial-audio-demo.livekit.io/) ([source](https://github.com/livekit-examples/spatial-audio))
    空间音频演示 ([源代码](https://github.com/livekit-examples/spatial-audio))

-   Livestreaming from OBS Studio ([source](https://github.com/livekit-examples/livestream))
    OBS Studio 直播示例 ([源代码](https://github.com/livekit-examples/livestream))

-   [AI voice assistant using ChatGPT](https://livekit.io/kitt) ([source](https://github.com/livekit-examples/kitt))
    使用 ChatGPT 的 AI 语音助手 ([源代码](https://github.com/livekit-examples/kitt))

## Ecosystem
## 生态系统

-   [Agents](https://github.com/livekit/agents): build real-time multimodal AI applications with programmable backend participants

    [智能代理](https://github.com/livekit/agents)：构建具有可编程后端参与者的实时多模态 AI 应用

-   [Egress](https://github.com/livekit/egress): record or multi-stream rooms and export individual tracks

    [媒体导出](https://github.com/livekit/egress)：录制或多流转发房间内容，支持导出单独轨道

-   [Ingress](https://github.com/livekit/ingress): ingest streams from external sources like RTMP, WHIP, HLS, or OBS Studio

    [媒体导入](https://github.com/livekit/ingress)：从 RTMP、WHIP、HLS 或 OBS Studio 等外部源接入流媒体

## SDKs & Tools
## SDK 与工具

### Client SDKs
### 客户端 SDK

Client SDKs enable your frontend to include interactive, multi-user experiences.

客户端 SDK 使您的前端能够包含交互式的多用户体验。

<table>
  <tr>
    <th>Language</th>
    <th>Repo</th>
    <th>
        <a href="https://docs.livekit.io/home/client/events/#declarative-ui" target="_blank" rel="noopener noreferrer">Declarative UI</a>
    </th>
    <th>Links</th>
  </tr>
  <tr>
    <th>语言</th>
    <th>仓库</th>
    <th>
        <a href="https://docs.livekit.io/home/client/events/#declarative-ui" target="_blank" rel="noopener noreferrer">声明式 UI</a>
    </th>
    <th>链接</th>
  </tr>
  <!-- BEGIN Template
  <tr>
    <td>Language</td>
    <td>
      <a href="" target="_blank" rel="noopener noreferrer"></a>
    </td>
    <td></td>
    <td></td>
  </tr>
  END -->
  <!-- JavaScript -->
  <tr>
    <td>JavaScript (TypeScript)</td>
    <td>
      <a href="https://github.com/livekit/client-sdk-js" target="_blank" rel="noopener noreferrer">client-sdk-js</a>
    </td>
    <td>
      <a href="https://github.com/livekit/livekit-react" target="_blank" rel="noopener noreferrer">React</a>
    </td>
    <td>
      <a href="https://docs.livekit.io/client-sdk-js/" target="_blank" rel="noopener noreferrer">文档</a>
      |
      <a href="https://github.com/livekit/client-sdk-js/tree/main/example" target="_blank" rel="noopener noreferrer">JS示例</a>
      |
      <a href="https://github.com/livekit/client-sdk-js/tree/main/example" target="_blank" rel="noopener noreferrer">React示例</a>
    </td>
  </tr>
  <!-- Swift -->
  <tr>
    <td>Swift (iOS / MacOS)</td>
    <td>
      <a href="https://github.com/livekit/client-sdk-swift" target="_blank" rel="noopener noreferrer">client-sdk-swift</a>
    </td>
    <td>Swift UI</td>
    <td>
      <a href="https://docs.livekit.io/client-sdk-swift/" target="_blank" rel="noopener noreferrer">文档</a>
      |
      <a href="https://github.com/livekit/client-example-swift" target="_blank" rel="noopener noreferrer">示例</a>
    </td>
  </tr>
  <!-- Kotlin -->
  <tr>
    <td>Kotlin (Android)</td>
    <td>
      <a href="https://github.com/livekit/client-sdk-android" target="_blank" rel="noopener noreferrer">client-sdk-android</a>
    </td>
    <td>Compose</td>
    <td>
      <a href="https://docs.livekit.io/client-sdk-android/index.html" target="_blank" rel="noopener noreferrer">docs</a>
      |
      <a href="https://github.com/livekit/client-sdk-android/tree/main/sample-app/src/main/java/io/livekit/android/sample" target="_blank" rel="noopener noreferrer">example</a>
      |
      <a href="https://github.com/livekit/client-sdk-android/tree/main/sample-app-compose/src/main/java/io/livekit/android/composesample" target="_blank" rel="noopener noreferrer">Compose example</a>
    </td>
  </tr>
<!-- Flutter -->
  <tr>
    <td>Flutter (all platforms)</td>
    <td>
      <a href="https://github.com/livekit/client-sdk-flutter" target="_blank" rel="noopener noreferrer">client-sdk-flutter</a>
    </td>
    <td>native</td>
    <td>
      <a href="https://docs.livekit.io/client-sdk-flutter/" target="_blank" rel="noopener noreferrer">docs</a>
      |
      <a href="https://github.com/livekit/client-sdk-flutter/tree/main/example" target="_blank" rel="noopener noreferrer">example</a>
    </td>
  </tr>
  <!-- Unity -->
  <tr>
    <td>Unity WebGL</td>
    <td>
      <a href="https://github.com/livekit/client-sdk-unity-web" target="_blank" rel="noopener noreferrer">client-sdk-unity-web</a>
    </td>
    <td></td>
    <td>
      <a href="https://livekit.github.io/client-sdk-unity-web/" target="_blank" rel="noopener noreferrer">docs</a>
    </td>
  </tr>
  <!-- React Native -->
  <tr>
    <td>React Native (beta)</td>
    <td>
      <a href="https://github.com/livekit/client-sdk-react-native" target="_blank" rel="noopener noreferrer">client-sdk-react-native</a>
    </td>
    <td>native</td>
    <td></td>
  </tr>
  <!-- Rust -->
  <tr>
    <td>Rust</td>
    <td>
      <a href="https://github.com/livekit/client-sdk-rust" target="_blank" rel="noopener noreferrer">client-sdk-rust</a>
    </td>
    <td></td>
    <td></td>
  </tr>
</table>

### Server SDKs
### 服务端 SDK

Server SDKs enable your backend to generate [access tokens](https://docs.livekit.io/home/get-started/authentication/),
call [server APIs](https://docs.livekit.io/reference/server/server-apis/), and
receive [webhooks](https://docs.livekit.io/home/server/webhooks/). In addition, the Go SDK includes client capabilities,
enabling you to build automations that behave like end-users.

服务端 SDK 使您的后端能够生成[访问令牌](https://docs.livekit.io/home/get-started/authentication/)，
调用[服务器 API](https://docs.livekit.io/reference/server/server-apis/)，
以及接收[网络钩子](https://docs.livekit.io/home/server/webhooks/)。此外，Go SDK 还包含客户端功能，
使您能够构建表现得像终端用户一样的自动化程序。

| Language | Repo | Docs |
| :------- | :---- | :---- |
| Go | [server-sdk-go](https://github.com/livekit/server-sdk-go) | [docs](https://pkg.go.dev/github.com/livekit/server-sdk-go) |
| JavaScript (TypeScript) | [server-sdk-js](https://github.com/livekit/server-sdk-js) | [docs](https://docs.livekit.io/server-sdk-js/) |
| Ruby | [server-sdk-ruby](https://github.com/livekit/server-sdk-ruby) | |
| Java (Kotlin) | [server-sdk-kotlin](https://github.com/livekit/server-sdk-kotlin) | |
| Python (community) | [python-sdks](https://github.com/livekit/python-sdks) | |
| PHP (community) | [agence104/livekit-server-sdk-php](https://github.com/agence104/livekit-server-sdk-php) | |

### Tools
### 工具

-   [CLI](https://github.com/livekit/livekit-cli) - command line interface & load tester
    命令行接口和负载测试工具

-   [Docker image](https://hub.docker.com/r/livekit/livekit-server)
    Docker 镜像

-   [Helm charts](https://github.com/livekit/livekit-helm)
    Helm 部署配置

## Install
## 安装

> [!TIP]
> We recommend installing [LiveKit CLI](https://github.com/livekit/livekit-cli) along with the server. It lets you access
> server APIs, create tokens, and generate test traffic.

> [!提示]
> 我们建议在安装服务器的同时安装 [LiveKit CLI](https://github.com/livekit/livekit-cli)。它允许您访问
> 服务器 API、创建令牌和生成测试流量。

The following will install LiveKit's media server:

### MacOS

```shell
brew install livekit
```

### Linux

```shell
curl -sSL https://get.livekit.io | bash
```

### Windows

Download the [latest release here](https://github.com/livekit/livekit/releases/latest)

## Getting Started
## 开始使用

### Starting LiveKit
### 启动 LiveKit

Start LiveKit in development mode by running `livekit-server --dev`. It'll use a placeholder API key/secret pair.

通过运行 `livekit-server --dev` 在开发模式下启动 LiveKit。它将使用一个占位的 API 密钥/密钥对。

```
API Key: devkey
API Secret: secret
```

To customize your setup for production, refer to our [deployment docs](https://docs.livekit.io/deploy/)

要为生产环境自定义设置，请参考我们的[部署文档](https://docs.livekit.io/deploy/)

### Creating access token
### 创建访问令牌

A user connecting to a LiveKit room requires an [access token](https://docs.livekit.io/home/get-started/authentication/#creating-a-token). Access
tokens (JWT) encode the user's identity and the room permissions they've been granted. You can generate a token with our
CLI:

用户连接到 LiveKit 房间需要一个[访问令牌](https://docs.livekit.io/home/get-started/authentication/#creating-a-token)。
访问令牌（JWT）编码了用户的身份和授予他们的房间权限。您可以使用我们的 CLI 生成令牌：

```shell
lk token create \
    --api-key devkey --api-secret secret \
    --join --room my-first-room --identity user1 \
    --valid-for 24h
```

### Test with example app
### 使用示例应用测试

Head over to our [example app](https://example.livekit.io) and enter a generated token to connect to your LiveKit
server. This app is built with our [React SDK](https://github.com/livekit/livekit-react).

访问我们的[示例应用](https://example.livekit.io)并输入生成的令牌以连接到您的 LiveKit 服务器。
这个应用是使用我们的 [React SDK](https://github.com/livekit/livekit-react) 构建的。

Once connected, your video and audio are now being published to your new LiveKit instance!

连接后，您的视频和音频就会被发布到您的新 LiveKit 实例中！

### Simulating a test publisher
### 模拟测试发布者

```shell
lk room join \
    --url ws://localhost:7880 \
    --api-key devkey --api-secret secret \
    --identity bot-user1 \
    --publish-demo \
    my-first-room
```

This command publishes a looped demo video to a room. Due to how the video clip was encoded (keyframes every 3s),
there's a slight delay before the browser has sufficient data to begin rendering frames. This is an artifact of the
simulation.

此命令将循环演示视频发布到房间。由于视频剪辑的编码方式（每3秒一个关键帧），
在浏览器有足够的数据开始渲染帧之前会有轻微的延迟。这是模拟过程中的一个特性。

## Deployment
## 部署

### Use LiveKit Cloud
### 使用 LiveKit Cloud

LiveKit Cloud is the fastest and most reliable way to run LiveKit. Every project gets free monthly bandwidth and
transcoding credits.

LiveKit Cloud 是运行 LiveKit 最快速和最可靠的方式。每个项目都可以获得免费的月度带宽和转码额度。

Sign up for [LiveKit Cloud](https://cloud.livekit.io/).

立即注册 [LiveKit Cloud](https://cloud.livekit.io/)。

### Self-host
### 自托管

Read our [deployment docs](https://docs.livekit.io/deploy/) for more information.

阅读我们的[部署文档](https://docs.livekit.io/deploy/)了解更多信息。

## Building from source
## 从源码构建

Pre-requisites:
前置要求：

-   Go 1.23+ is installed
    已安装 Go 1.23+ 版本
-   GOPATH/bin is in your PATH
    GOPATH/bin 已添加到系统 PATH 中

Then run
然后运行

```shell
git clone https://github.com/livekit/livekit
cd livekit
./bootstrap.sh
mage
```

## Contributing
## 贡献代码

We welcome your contributions toward improving LiveKit! Please join us
[on Slack](http://livekit.io/join-slack) to discuss your ideas and/or PRs.

我们欢迎您为改进 LiveKit 做出贡献！请加入我们的 [Slack 社区](http://livekit.io/join-slack)讨论您的想法和/或 PR。

## License
## 许可证

LiveKit server is licensed under Apache License v2.0.

LiveKit 服务器采用 Apache License v2.0 许可证。

<!--BEGIN_REPO_NAV-->
<br/><table>
<thead><tr><th colspan="2">LiveKit Ecosystem</th></tr></thead>
<tbody>
<tr><td>LiveKit SDKs</td><td><a href="https://github.com/livekit/client-sdk-js">Browser</a> · <a href="https://github.com/livekit/client-sdk-swift">iOS/macOS/visionOS</a> · <a href="https://github.com/livekit/client-sdk-android">Android</a> · <a href="https://github.com/livekit/client-sdk-flutter">Flutter</a> · <a href="https://github.com/livekit/client-sdk-react-native">React Native</a> · <a href="https://github.com/livekit/rust-sdks">Rust</a> · <a href="https://github.com/livekit/node-sdks">Node.js</a> · <a href="https://github.com/livekit/python-sdks">Python</a> · <a href="https://github.com/livekit/client-sdk-unity">Unity</a> · <a href="https://github.com/livekit/client-sdk-unity-web">Unity (WebGL)</a></td></tr><tr></tr>
<tr><td>Server APIs</td><td><a href="https://github.com/livekit/node-sdks">Node.js</a> · <a href="https://github.com/livekit/server-sdk-go">Golang</a> · <a href="https://github.com/livekit/server-sdk-ruby">Ruby</a> · <a href="https://github.com/livekit/server-sdk-kotlin">Java/Kotlin</a> · <a href="https://github.com/livekit/python-sdks">Python</a> · <a href="https://github.com/livekit/rust-sdks">Rust</a> · <a href="https://github.com/agence104/livekit-server-sdk-php">PHP (community)</a> · <a href="https://github.com/pabloFuente/livekit-server-sdk-dotnet">.NET (community)</a></td></tr><tr></tr>
<tr><td>UI Components</td><td><a href="https://github.com/livekit/components-js">React</a> · <a href="https://github.com/livekit/components-android">Android Compose</a> · <a href="https://github.com/livekit/components-swift">SwiftUI</a></td></tr><tr></tr>
<tr><td>Agents Frameworks</td><td><a href="https://github.com/livekit/agents">Python</a> · <a href="https://github.com/livekit/agents-js">Node.js</a> · <a href="https://github.com/livekit/agent-playground">Playground</a></td></tr><tr></tr>
<tr><td>Services</td><td><b>LiveKit server</b> · <a href="https://github.com/livekit/egress">Egress</a> · <a href="https://github.com/livekit/ingress">Ingress</a> · <a href="https://github.com/livekit/sip">SIP</a></td></tr><tr></tr>
<tr><td>Resources</td><td><a href="https://docs.livekit.io">Docs</a> · <a href="https://github.com/livekit-examples">Example apps</a> · <a href="https://livekit.io/cloud">Cloud</a> · <a href="https://docs.livekit.io/home/self-hosting/deployment">Self-hosting</a> · <a href="https://github.com/livekit/livekit-cli">CLI</a></td></tr>
</tbody>
</table>
<!--END_REPO_NAV-->
