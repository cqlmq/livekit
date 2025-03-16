# LiveKit Config 包教程

## 1. 包的功能

config 包是 LiveKit 服务器的配置管理包，主要负责：

- 配置的加载与解析
- 配置验证
- 配置默认值管理
- 多种配置源支持（YAML、环境变量、命令行）

## 2. 配置加载优先级

配置按以下优先级从高到低加载：

1. 命令行参数
2. 环境变量（LIVEKIT_*）
3. YAML 配置文件
4. 默认配置值

## 3. 核心结构

### 3.1 Config 结构体

主配置结构体，包含所有服务器配置项。重要字段包括：

#### 基础配置
- Port: 服务器监听端口
- BindAddresses: 服务器绑定地址列表
- Development: 是否为开发模式
- Region: 服务器区域标识

#### 监控和指标
- Prometheus: Prometheus监控配置
- Metric: 指标收集配置

#### 核心服务配置
- RTC: WebRTC相关配置
- RTC.TURNServers: 客户端可用的外部TURN服务器列表
- Redis: Redis服务配置
- Audio: 音频处理配置
- Video: 视频处理配置
- Room: 房间管理配置
- TURN: LiveKit内置TURN服务器配置

#### 集成服务配置
- Ingress: 入口服务配置
- SIP: SIP服务配置
- WebHook: WebHook配置
- NodeSelector: 节点选择器配置
- SignalRelay: 信号中继配置
- PSRPC: RPC服务配置

#### 安全和限制
- KeyFile: API密钥文件路径
- Keys: API密钥映射表
- Limit: 系统资源限制配置

#### 日志配置
- Logging: 日志配置
- LogLevel: 日志级别(已废弃)

### 3.2 主要子配置结构

- RTCConfig: WebRTC相关配置，包含ICE、STUN/TURN等设置
- RoomConfig: 房间配置，包含房间创建、编解码器等设置
- LoggingConfig: 日志配置，包含日志级别等设置
- TURNConfig: TURN服务器配置
- LimitConfig: 系统限制配置，包含各种资源限制

#### TURNConfig 详解

LiveKit 内置的 TURN 服务器配置包含以下字段：

```yaml
turn:
  # 基本配置
  enabled: true        # 是否启用内置TURN服务器
  domain: turn.example.com   # TURN服务器域名

  # TLS相关配置
  cert_file: /path/to/cert.pem   # TLS证书文件路径
  key_file: /path/to/key.pem     # TLS私钥文件路径
  external_tls: false    # 是否使用外部TLS(如nginx)

  # 端口配置
  tls_port: 5349    # TURN-TLS端口(默认5349)
  udp_port: 3478    # TURN-UDP端口(默认3478)

  # 中继端口范围
  relay_range_start: 30000  # 中继端口范围起始
  relay_range_end: 40000    # 中继端口范围结束
```

说明：
1. TURN服务器的作用
   - 作为媒体中继服务器
   - 帮助穿透NAT和防火墙
   - 提供回退机制当P2P连接失败时

2. 配置建议
   - 生产环境建议启用TLS
   - 确保端口范围足够大以支持多个连接
   - 如果使用外部负载均衡器，设置external_tls为true

3. 端口说明
   - TLS端口(5349)：用于加密的TURN连接
   - UDP端口(3478)：用于标准TURN连接
   - 中继端口：用于实际的媒体传输

4. 安全性考虑
   - 证书文件权限设置正确
   - 合理配置中继端口范围
   - 考虑使用防火墙保护TURN服务器

## 4. 使用方法

### 4.1 基本用法

1. 创建新配置：
   ```go
   conf, err := config.NewConfig(yamlString, strictMode, cliContext, baseFlags)
   ```

2. 验证API密钥：
   ```go
   err := conf.ValidateKeys()
   ```

3. 获取限制配置：
   ```go
   limits := conf.GetLimitConfig()
   ```

### 4.2 配置验证方法

- CheckRoomNameLength: 检查房间名称长度
- CheckMetadataSize: 检查元数据大小
- CheckParticipantNameLength: 检查参与者名称长度
- CheckAttributesSize: 检查属性大小

## 5. 最佳实践

1. 使用严格模式以捕获配置错误
2. 通过环境变量或密钥文件管理敏感配置
3. 始终验证API密钥的安全性
4. 遵循配置限制确保系统稳定性
5. 使用omitempty YAML标签避免序列化零值

## 6. 配置示例

### 6.1 基本YAML配置示例

```yaml
# 基础服务配置
port: 7880
bind_addresses:
  - 0.0.0.0
region: us-west
development: false

# 监控配置
prometheus:
  port: 6789
  username: admin
  password: secret

# Redis配置
redis:
  address: localhost:6379

# 房间配置
room:
  auto_create: true
  enabled_codecs:
    - mime: audio/opus
    - mime: video/h264
    - mime: video/vp8
  max_participants: 100
  empty_timeout: 300

# 日志配置
logging:
  level: info
  pion_level: error

# 限制配置
limit:
  num_tracks: 100
  bytes_per_sec: 1048576
  subscription_limit_video: 25
  subscription_limit_audio: 50

# TURN服务器配置
turn:
  enabled: true
  domain: turn.example.com
  tls_port: 5349
  udp_port: 3478
  relay_range_start: 30000
  relay_range_end: 40000

# RTC配置中的外部TURN服务器
rtc:
  turn_servers:
    - host: stun.l.google.com
      port: 19302
      protocol: udp
    - host: turn.example.com
      port: 3478
      protocol: tcp
      username: turnuser
      credential: turnpass

# WebHook配置
webhook:
  urls:
    - https://webhook.example.com/livekit
  api_key: webhook_secret

# 节点选择器配置
node_selector:
  kind: random
  regions:
    - name: us-west
      lat: 37.77493
      lon: -122.41942
```

### 6.2 环境变量配置示例

```bash
export LIVEKIT_PORT=7880
export LIVEKIT_REDIS_ADDRESS=localhost:6379
export LIVEKIT_KEYS="apikey1: secret1"
```

#### 使用环境变量的优势

1. 安全性提升
   - 避免敏感信息写入配置文件
   - 防止密钥被意外提交到代码仓库
   - 支持密钥管理服务集成

2. 部署便利性
   - 环境隔离：开发/测试/生产使用不同密钥
   - 容器化支持：适合 Docker/Kubernetes 部署
   - 云平台集成：配合云服务的密钥管理

3. 运维管理优势
   - 支持动态更新：无需重启即可更新密钥
   - 便于密钥轮换：定期更换密钥提高安全性
   - 审计跟踪：密钥使用更容易追踪

#### 最佳实践

```bash
# 生产环境使用随机生成的强密钥
export LIVEKIT_KEYS="$(openssl rand -hex 16): $(openssl rand -hex 32)"

# 或使用密钥文件
export LIVEKIT_KEY_FILE=/path/to/secure/keys.yaml

# 多个密钥支持
export LIVEKIT_KEYS="key1: secret1, key2: secret2"
```

#### 多节点部署注意事项

1. Keys 配置
   - 所有节点必须使用相同的 API Keys
   - 可以通过环境变量或配置文件保持一致
   - 修改 Keys 时需要同步更新所有节点

2. 部署建议
   - 使用配置管理工具统一管理密钥
   - 考虑使用密钥管理服务(如 Vault、AWS KMS)
   - 实现自动化的密钥同步机制

3. 安全考虑
   - 在节点间传输密钥时使用加密通道
   - 定期轮换密钥并确保同步更新
   - 保持密钥使用记录便于审计

示例配置管理方式：
```yaml
# 通过共享配置文件
key_file: /shared/storage/livekit_keys.yaml

# 或通过环境变量
LIVEKIT_KEYS: ${SHARED_LIVEKIT_KEYS}

# 或通过密钥管理服务
key_file: vault://secrets/livekit/keys
```

## 7. 安全性考虑

1. API密钥管理
   - 密钥长度至少32字符
   - 使用环境变量或密钥文件存储
   - 正确设置密钥文件权限

2. 限制配置
   - 设置合理的资源限制
   - 配置房间人数上限
   - 限制元数据大小

3. 生产环境配置
   - 禁用开发模式
   - 使用TLS加密
   - 配置防火墙规则

## 8. 调试指南

### 8.1 开启调试日志

```yaml
development: true
logging:
  level: debug
  pion_level: debug
```

### 8.2 常见问题排查

1. 配置未生效
   - 检查配置加载优先级
   - 验证环境变量名称
   - 确认YAML语法

2. 密钥验证失败
   - 检查密钥文件权限
   - 验证密钥格式
   - 确认密钥长度

3. 限制验证失败
   - 检查限制配置值
   - 验证实际使用值
   - 调整限制参数

## 9. 高级特性

### 9.1 命令行参数生成

```go
flags, err := config.GenerateCLIFlags(existingFlags, hidden)
```

### 9.2 配置更新

```go
err := conf.updateFromCLI(cliContext, baseFlags)
```

### 9.3 YAML标签检查

```go
err := configtest.CheckYAMLTags(Config{})
```

## 10. 参考资料

- [LiveKit 官方文档](https://docs.livekit.io)
- [配置示例](https://github.com/livekit/livekit-server/tree/master/config)
- [YAML 规范](https://yaml.org/spec/1.2/spec.html)