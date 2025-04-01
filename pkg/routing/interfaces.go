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

package routing

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
	"go.uber.org/atomic"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"

	"github.com/livekit/livekit-server/pkg/utils"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/rpc"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

// MessageSink is an abstraction for writing protobuf messages and having them read by a MessageSource,
// potentially on a different node via a transport
// 消息发送器 是一个抽象接口，用于写入 protobuf 消息并让 MessageSource 读取这些消息，
// 可能通过传输层在不同节点上进行
//
//counterfeiter:generate . MessageSink
type MessageSink interface {
	WriteMessage(msg proto.Message) error
	IsClosed() bool
	Close()
	ConnectionID() livekit.ConnectionID
}

// ----------

// 空消息发送器
type NullMessageSink struct {
	connID   livekit.ConnectionID
	isClosed atomic.Bool
}

func NewNullMessageSink(connID livekit.ConnectionID) *NullMessageSink {
	return &NullMessageSink{
		connID: connID,
	}
}

func (n *NullMessageSink) WriteMessage(_msg proto.Message) error {
	return nil
}

func (n *NullMessageSink) IsClosed() bool {
	return n.isClosed.Load()
}

func (n *NullMessageSink) Close() {
	n.isClosed.Store(true)
}

func (n *NullMessageSink) ConnectionID() livekit.ConnectionID {
	return n.connID
}

// ------------------------------------------------

//counterfeiter:generate . MessageSource
type MessageSource interface {
	// ReadChan exposes a one way channel to make it easier to use with select
	// 暴露一个单向通道，方便与 select 一起使用
	ReadChan() <-chan proto.Message
	IsClosed() bool
	Close()
	ConnectionID() livekit.ConnectionID
}

// ----------

// 空消息源
type NullMessageSource struct {
	connID   livekit.ConnectionID
	msgChan  chan proto.Message
	isClosed atomic.Bool
}

func NewNullMessageSource(connID livekit.ConnectionID) *NullMessageSource {
	return &NullMessageSource{
		connID:  connID,
		msgChan: make(chan proto.Message),
	}
}

func (n *NullMessageSource) ReadChan() <-chan proto.Message {
	return n.msgChan
}

func (n *NullMessageSource) IsClosed() bool {
	return n.isClosed.Load()
}

func (n *NullMessageSource) Close() {
	if !n.isClosed.Swap(true) {
		close(n.msgChan)
	}
}

func (n *NullMessageSource) ConnectionID() livekit.ConnectionID {
	return n.connID
}

// ------------------------------------------------

// Router allows multiple nodes to coordinate the participant session
//
//counterfeiter:generate . Router
type Router interface {
	MessageRouter // 消息路由器

	RegisterNode() error    // 注册节点
	UnregisterNode() error  // 注销节点
	RemoveDeadNodes() error // 移除死节点

	ListNodes() ([]*livekit.Node, error) // 列出节点

	GetNodeForRoom(ctx context.Context, roomName livekit.RoomName) (*livekit.Node, error)       // 获取房间节点
	SetNodeForRoom(ctx context.Context, roomName livekit.RoomName, nodeId livekit.NodeID) error // 设置房间节点
	ClearRoomState(ctx context.Context, roomName livekit.RoomName) error                        // 清除房间状态

	GetRegion() string // 获取区域

	Start() error // 启动
	Drain()       // 排空
	Stop()        // 停止
}

// 开始参与者信号结果
type StartParticipantSignalResults struct {
	ConnectionID        livekit.ConnectionID // 连接 ID
	RequestSink         MessageSink          // 发送器
	ResponseSource      MessageSource        // 接收器
	NodeID              livekit.NodeID       // 节点 ID
	NodeSelectionReason string               // 节点选择原因
}

// 消息路由器
type MessageRouter interface {
	// CreateRoom starts an rtc room
	// 创建一个 rtc 房间
	CreateRoom(ctx context.Context, req *livekit.CreateRoomRequest) (res *livekit.Room, err error)
	// StartParticipantSignal participant signal connection is ready to start
	// 开始参与者信号连接
	StartParticipantSignal(ctx context.Context, roomName livekit.RoomName, pi ParticipantInit) (res StartParticipantSignalResults, err error)
}

// 创建路由器
func CreateRouter(
	rc redis.UniversalClient, // redis 客户端
	node LocalNode, // 本地节点
	signalClient SignalClient, // 信令客户端
	roomManagerClient RoomManagerClient, // 房间管理客户端
	kps rpc.KeepalivePubSub, // 保持活跃的发布订阅
) Router {
	lr := NewLocalRouter(node, signalClient, roomManagerClient)

	if rc != nil {
		return NewRedisRouter(lr, rc, kps)
	}

	// local routing and store
	logger.Infow("using single-node routing")
	return lr
}

// ------------------------------------------------

// 参与者初始化
type ParticipantInit struct {
	Identity             livekit.ParticipantIdentity // 参与者身份
	Name                 livekit.ParticipantName     // 参与者名称
	Reconnect            bool                        // 是否重连
	ReconnectReason      livekit.ReconnectReason     // 重连原因
	AutoSubscribe        bool                        // 是否自动订阅
	Client               *livekit.ClientInfo         // 客户端信息
	Grants               *auth.ClaimGrants           // 授权
	Region               string                      // 区域
	AdaptiveStream       bool                        // 自适应流
	ID                   livekit.ParticipantID       // 参与者 ID
	SubscriberAllowPause *bool                       // 订阅者允许暂停
	DisableICELite       bool                        // 禁用 ICE Lite
	CreateRoom           *livekit.CreateRoomRequest  // 创建房间请求
}

// 参与者初始化日志对象
func (pi *ParticipantInit) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if pi == nil {
		return nil
	}

	logBoolPtr := func(prop string, val *bool) {
		if val == nil {
			e.AddString(prop, "not-set")
		} else {
			e.AddBool(prop, *val)
		}
	}

	e.AddString("Identity", string(pi.Identity))
	logBoolPtr("Reconnect", &pi.Reconnect)
	e.AddString("ReconnectReason", pi.ReconnectReason.String())
	logBoolPtr("AutoSubscribe", &pi.AutoSubscribe)
	e.AddObject("Client", logger.Proto(utils.ClientInfoWithoutAddress(pi.Client)))
	e.AddObject("Grants", pi.Grants)
	e.AddString("Region", pi.Region)
	logBoolPtr("AdaptiveStream", &pi.AdaptiveStream)
	e.AddString("ID", string(pi.ID))
	logBoolPtr("SubscriberAllowPause", pi.SubscriberAllowPause)
	logBoolPtr("DisableICELite", &pi.DisableICELite)
	e.AddObject("CreateRoom", logger.Proto(pi.CreateRoom))
	return nil
}

// 转换为开始会话
func (pi *ParticipantInit) ToStartSession(roomName livekit.RoomName, connectionID livekit.ConnectionID) (*livekit.StartSession, error) {
	claims, err := json.Marshal(pi.Grants)
	if err != nil {
		return nil, err
	}

	ss := &livekit.StartSession{
		RoomName: string(roomName),
		Identity: string(pi.Identity),
		Name:     string(pi.Name),
		// connection id is to allow the RTC node to identify where to route the message back to
		ConnectionId:    string(connectionID),
		Reconnect:       pi.Reconnect,
		ReconnectReason: pi.ReconnectReason,
		AutoSubscribe:   pi.AutoSubscribe,
		Client:          pi.Client,
		GrantsJson:      string(claims),
		AdaptiveStream:  pi.AdaptiveStream,
		ParticipantId:   string(pi.ID),
		DisableIceLite:  pi.DisableICELite,
		CreateRoom:      pi.CreateRoom,
	}
	if pi.SubscriberAllowPause != nil {
		subscriberAllowPause := *pi.SubscriberAllowPause
		ss.SubscriberAllowPause = &subscriberAllowPause
	}

	return ss, nil
}

// 从开始会话转换为参与者初始化
func ParticipantInitFromStartSession(ss *livekit.StartSession, region string) (*ParticipantInit, error) {
	claims := &auth.ClaimGrants{}
	if err := json.Unmarshal([]byte(ss.GrantsJson), claims); err != nil {
		return nil, err
	}

	pi := &ParticipantInit{
		Identity:        livekit.ParticipantIdentity(ss.Identity),
		Name:            livekit.ParticipantName(ss.Name),
		Reconnect:       ss.Reconnect,
		ReconnectReason: ss.ReconnectReason,
		Client:          ss.Client,
		AutoSubscribe:   ss.AutoSubscribe,
		Grants:          claims,
		Region:          region,
		AdaptiveStream:  ss.AdaptiveStream,
		ID:              livekit.ParticipantID(ss.ParticipantId),
		DisableICELite:  ss.DisableIceLite,
		CreateRoom:      ss.CreateRoom,
	}
	if ss.SubscriberAllowPause != nil {
		subscriberAllowPause := *ss.SubscriberAllowPause
		pi.SubscriberAllowPause = &subscriberAllowPause
	}

	// TODO: clean up after 1.7 eol
	if pi.CreateRoom == nil {
		pi.CreateRoom = &livekit.CreateRoomRequest{
			Name: ss.RoomName,
		}
	}

	return pi, nil
}
