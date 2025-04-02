
# LiveKit 源码学习方案 - 服务层篇

## 第一阶段：基础架构和概念（1-2周）

### 1. 基础组件
- `interfaces.go` - 了解核心接口定义
- `utils.go` - 熟悉通用工具函数
- `errors.go` - 了解错误处理机制
- `wire.md` 和 `tutorial.md` - 理解架构设计和教程

### 2. 身份验证
- `auth.go` - 学习授权机制
- `basic_auth.go` - 基本认证实现

### 3. 通信基础
- `wsprotocol.go` - WebSocket协议处理
- `signal.go` - 信令处理机制

## 第二阶段：核心服务实现（2-3周）

### 1. 房间管理
- `roomallocator.go` - 房间分配机制
- `roomservice.go` - 房间服务API
- `roommanager.go` - 房间管理核心逻辑
  - 重点关注：房间创建、参与者加入管理、消息路由

### 2. 实时通信
- `rtcservice.go` - WebRTC服务核心
  - 重点关注：WebSocket连接建立、信令交换流程、媒体流转发

### 3. 存储实现
- `localstore.go` - 本地存储
- `redisstore.go` - Redis存储实现
  - 重点：房间状态持久化、参与者信息管理

### 4. 服务器实现
- `server.go` - HTTP服务器实现
- `twirp.go` - RPC服务实现

## 第三阶段：扩展功能（2-3周）

### 1. 多媒体处理
- `turn.go` - TURN服务实现
- `ioservice.go` - I/O服务基础

### 2. 特殊功能
- `ingress.go` - 外部媒体输入
- `egress.go` - 媒体录制和广播
- `sip.go` - SIP协议集成（如需要）

### 3. 高级管理
- `agentservice.go` - Agent服务
- `agent_dispatch_service.go` - Agent调度

## 第四阶段：依赖注入与集成（1周）

### 1. 依赖注入
- `wire.go` - 依赖关系定义
- `wire_gen.go` - 生成的依赖注入代码

### 2. 测试与实践
- 各种 `*_test.go` 文件 - 了解测试方法和用例

## 学习建议

### 推荐学习顺序
1. 先整体浏览 interfaces.go 了解接口定义
2. 按照上述阶段逐步深入
3. 每学习一个文件，运行相关测试理解行为

### 实践项目
1. **简单项目**：实现一个基本的房间创建和加入功能
2. **中级项目**：添加自定义认证和用户管理
3. **高级项目**：实现一个自定义的媒体处理插件

### 学习工具
1. 使用代码阅读工具（如VSCode + Go插件）
2. 在本地设置调试环境
3. 维护学习笔记，记录关键概念和流程

### 重点关注
1. **rtcservice.go** 中的 WebSocket 连接管理
2. **roommanager.go** 中的参与者和房间状态管理
3. 信令流程和消息路由机制
4. 存储层设计及状态一致性保证
5. 错误处理和恢复机制

祝您学习顺利！如果有特定需求或疑问，可以随时调整学习计划。
