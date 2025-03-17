// Copyright 2023 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build wireinject
// +build wireinject

package service

import (
	"fmt"
	"os"

	"github.com/google/wire"
	"github.com/pion/turn/v4"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"

	"github.com/livekit/livekit-server/pkg/agent"
	"github.com/livekit/livekit-server/pkg/clientconfiguration"
	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/pkg/sfu"
	"github.com/livekit/livekit-server/pkg/telemetry"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	redisLiveKit "github.com/livekit/protocol/redis"
	"github.com/livekit/protocol/rpc"
	"github.com/livekit/protocol/utils"
	"github.com/livekit/protocol/webhook"
	"github.com/livekit/psrpc"
)

// InitializeServer 初始化Livekit服务器
// changed by limengqiu 2025-03-16 尝试修改redis的参数方式，测试通过
// 采用：wire ./pkg/service 重新生成wire_gen.go文件
// 心得体会：
// 1、函数需要的参数，必需在wire.Build()中添加，不能使用对象的成员变量
func InitializeServer(conf *config.Config, currentNode routing.LocalNode) (*LivekitServer, error) {
	wire.Build(
		wire.Bind(new(routing.MessageRouter), new(routing.Router)), // 绑定消息路由
		wire.Bind(new(livekit.RoomService), new(*RoomService)),     // 绑定房间服务
		wire.Bind(new(ServiceStore), new(ObjectStore)),             // 绑定服务存储
		wire.Bind(new(IOClient), new(*IOInfoService)),              // 绑定IO客户端
		getNodeID,                               // 获取节点ID
		getRedisConfig,                          // 获取Redis配置
		createRedisClient,                       // 创建Redis客户端
		createStore,                             // 创建存储
		createKeyProvider,                       // 创建密钥提供者
		createWebhookNotifier,                   // 创建Webhook通知器
		createClientConfiguration,               // 创建客户端配置
		createForwardStats,                      // 创建转发统计
		routing.CreateRouter,                    // 创建路由
		getLimitConf,                            // 获取限制配置
		config.DefaultAPIConfig,                 // 默认API配置
		telemetry.NewAnalyticsService,           // 创建分析服务
		telemetry.NewTelemetryService,           // 创建Telemetry服务
		getMessageBus,                           // 获取消息总线
		NewIOInfoService,                        // 创建IO信息服务
		rpc.NewEgressClient,                     // 创建Egress客户端
		rpc.NewIngressClient,                    // 创建Ingress客户端
		getEgressStore,                          // 获取Egress存储
		NewEgressLauncher,                       // 创建Egress启动器
		NewEgressService,                        // 创建Egress服务
		getIngressStore,                         // 获取Ingress存储
		getIngressConfig,                        // 获取Ingress配置
		NewIngressService,                       // 创建Ingress服务
		rpc.NewSIPClient,                        // 创建SIP客户端
		getSIPStore,                             // 获取SIP存储
		getSIPConfig,                            // 获取SIP配置
		NewSIPService,                           // 创建SIP服务
		NewRoomAllocator,                        // 创建房间分配器
		NewRoomService,                          // 创建房间服务
		NewRTCService,                           // 创建RTC服务
		NewAgentService,                         // 创建Agent服务
		NewAgentDispatchService,                 // 创建Agent调度服务
		agent.NewAgentClient,                    // 创建Agent客户端
		getAgentStore,                           // 获取Agent存储
		getSignalRelayConfig,                    // 获取信号Relay配置
		NewDefaultSignalServer,                  // 创建默认信号服务器
		routing.NewSignalClient,                 // 创建信号客户端
		getRoomConfig,                           // 获取房间配置
		routing.NewRoomManagerClient,            // 创建房间管理客户端
		rpc.NewKeepalivePubSub,                  // 创建Keepalive发布订阅
		getPSRPCConfig,                          // 获取PSRPC配置
		getPSRPCClientParams,                    // 获取PSRPC客户端参数
		rpc.NewTopicFormatter,                   // 创建Topic格式化器
		rpc.NewTypedRoomClient,                  // 创建TypedRoom客户端
		rpc.NewTypedParticipantClient,           // 创建TypedParticipant客户端
		rpc.NewTypedAgentDispatchInternalClient, // 创建TypedAgentDispatchInternal客户端
		NewLocalRoomManager,                     // 创建LocalRoomManager
		NewTURNAuthHandler,                      // 创建TURNAuthHandler
		getTURNAuthHandlerFunc,                  // 获取TURNAuthHandlerFunc
		newInProcessTurnServer,                  // 创建InProcessTurnServer
		utils.NewDefaultTimedVersionGenerator,   // 创建DefaultTimedVersionGenerator
		NewLivekitServer,                        // 创建LivekitServer
	)
	return &LivekitServer{}, nil
}

func InitializeRouter(conf *config.Config, currentNode routing.LocalNode) (routing.Router, error) {
	wire.Build(
		createRedisClient,
		getNodeID,
		getRedisConfig,
		getMessageBus,
		getSignalRelayConfig,
		getPSRPCConfig,
		getPSRPCClientParams,
		routing.NewSignalClient,
		getRoomConfig,
		routing.NewRoomManagerClient,
		rpc.NewKeepalivePubSub,
		routing.CreateRouter,
	)

	return nil, nil
}

func getNodeID(currentNode routing.LocalNode) livekit.NodeID {
	return currentNode.NodeID()
}

func createKeyProvider(conf *config.Config) (auth.KeyProvider, error) {
	// prefer keyfile if set
	if conf.KeyFile != "" {
		var otherFilter os.FileMode = 0007
		if st, err := os.Stat(conf.KeyFile); err != nil {
			return nil, err
		} else if st.Mode().Perm()&otherFilter != 0000 {
			return nil, fmt.Errorf("key file others permissions must be set to 0")
		}
		f, err := os.Open(conf.KeyFile)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = f.Close()
		}()
		decoder := yaml.NewDecoder(f)
		if err = decoder.Decode(conf.Keys); err != nil {
			return nil, err
		}
	}

	if len(conf.Keys) == 0 {
		return nil, errors.New("one of key-file or keys must be provided in order to support a secure installation")
	}

	return auth.NewFileBasedKeyProviderFromMap(conf.Keys), nil
}

func createWebhookNotifier(conf *config.Config, provider auth.KeyProvider) (webhook.QueuedNotifier, error) {
	wc := conf.WebHook
	if len(wc.URLs) == 0 {
		return nil, nil
	}
	secret := provider.GetSecret(wc.APIKey)
	if secret == "" {
		return nil, ErrWebHookMissingAPIKey
	}

	return webhook.NewDefaultNotifier(wc.APIKey, secret, wc.URLs), nil
}

// createRedisClient 获取Redis客户端,
func createRedisClient(conf *redisLiveKit.RedisConfig) (redis.UniversalClient, error) {
	if !conf.IsConfigured() {
		logger.Debugw("redis not configured, using local store")
		return nil, nil
	}
	return redisLiveKit.GetRedisClient(conf)
}

// createStore 创建存储,
func createStore(rc redis.UniversalClient) ObjectStore {
	if rc != nil {
		return NewRedisStore(rc)
	}
	return NewLocalStore()
}

// getMessageBus 获取消息总线
// 另一个项目了解总线的原理与功能
func getMessageBus(rc redis.UniversalClient) psrpc.MessageBus {
	if rc == nil {
		return psrpc.NewLocalMessageBus()
	}
	return psrpc.NewRedisMessageBus(rc)
}

func getEgressStore(s ObjectStore) EgressStore {
	switch store := s.(type) {
	case *RedisStore:
		return store
	default:
		return nil
	}
}

func getIngressStore(s ObjectStore) IngressStore {
	switch store := s.(type) {
	case *RedisStore:
		return store
	default:
		return nil
	}
}

func getAgentStore(s ObjectStore) AgentStore {
	switch store := s.(type) {
	case *RedisStore:
		return store
	case *LocalStore:
		return store
	default:
		return nil
	}
}

func getIngressConfig(conf *config.Config) *config.IngressConfig {
	return &conf.Ingress
}

func getSIPStore(s ObjectStore) SIPStore {
	switch store := s.(type) {
	case *RedisStore:
		return store
	default:
		return nil
	}
}

func getSIPConfig(conf *config.Config) *config.SIPConfig {
	return &conf.SIP
}

func getRedisConfig(conf *config.Config) *redisLiveKit.RedisConfig {
	return &conf.Redis
}

func createClientConfiguration() clientconfiguration.ClientConfigurationManager {
	return clientconfiguration.NewStaticClientConfigurationManager(clientconfiguration.StaticConfigurations)
}

func getLimitConf(config *config.Config) config.LimitConfig {
	return config.GetLimitConfig()
}

func getRoomConfig(config *config.Config) config.RoomConfig {
	return config.Room
}

func getSignalRelayConfig(config *config.Config) config.SignalRelayConfig {
	return config.SignalRelay
}

func getPSRPCConfig(config *config.Config) rpc.PSRPCConfig {
	return config.PSRPC
}

func getPSRPCClientParams(config rpc.PSRPCConfig, bus psrpc.MessageBus) rpc.ClientParams {
	return rpc.NewClientParams(config, bus, logger.GetLogger(), rpc.PSRPCMetricsObserver{})
}

func createForwardStats(conf *config.Config) *sfu.ForwardStats {
	if conf.RTC.ForwardStats.SummaryInterval == 0 || conf.RTC.ForwardStats.ReportInterval == 0 || conf.RTC.ForwardStats.ReportWindow == 0 {
		return nil
	}
	return sfu.NewForwardStats(conf.RTC.ForwardStats.SummaryInterval, conf.RTC.ForwardStats.ReportInterval, conf.RTC.ForwardStats.ReportWindow)
}

func newInProcessTurnServer(conf *config.Config, authHandler turn.AuthHandler) (*turn.Server, error) {
	return NewTurnServer(conf, authHandler, false)
}
