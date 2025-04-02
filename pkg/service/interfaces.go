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

package service

import (
	"context"
	"time"

	"github.com/livekit/protocol/livekit"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

// encapsulates CRUD operations for room settings // 封装了房间设置的CRUD操作
// LocalStore和RedisStore实现了这个接口
//
//counterfeiter:generate . ObjectStore
type ObjectStore interface {
	ServiceStore

	// enable locking on a specific room to prevent race
	// returns a (lock uuid, error)
	// 启用对特定房间的锁定，以防止竞争
	// 返回一个(锁定UUID, 错误)
	LockRoom(ctx context.Context, roomName livekit.RoomName, duration time.Duration) (string, error)
	// 解锁房间
	UnlockRoom(ctx context.Context, roomName livekit.RoomName, uid string) error
	// 存储房间的内部信息
	StoreRoom(ctx context.Context, room *livekit.Room, internal *livekit.RoomInternal) error
	// 存储参与者
	StoreParticipant(ctx context.Context, roomName livekit.RoomName, participant *livekit.ParticipantInfo) error
	// 删除参与者
	DeleteParticipant(ctx context.Context, roomName livekit.RoomName, identity livekit.ParticipantIdentity) error
}

// 服务存储接口
// 提供房间的加载、删除、列表等操作
//
//counterfeiter:generate . ServiceStore
type ServiceStore interface {
	// 加载房间
	LoadRoom(ctx context.Context, roomName livekit.RoomName, includeInternal bool) (*livekit.Room, *livekit.RoomInternal, error)
	// 删除房间
	DeleteRoom(ctx context.Context, roomName livekit.RoomName) error

	// ListRooms returns currently active rooms. if names is not nil, it'll filter and return
	// only rooms that match
	// 列出当前活跃的房间。如果names不为nil，它将过滤并返回
	// 只匹配的房间
	ListRooms(ctx context.Context, roomNames []livekit.RoomName) ([]*livekit.Room, error)
	// 加载参与者
	LoadParticipant(ctx context.Context, roomName livekit.RoomName, identity livekit.ParticipantIdentity) (*livekit.ParticipantInfo, error)
	// 列出参与者
	ListParticipants(ctx context.Context, roomName livekit.RoomName) ([]*livekit.ParticipantInfo, error)
}

// 出口存储接口
// 提供出口的存储、加载、列表等操作
//
//counterfeiter:generate . EgressStore
type EgressStore interface {
	StoreEgress(ctx context.Context, info *livekit.EgressInfo) error
	LoadEgress(ctx context.Context, egressID string) (*livekit.EgressInfo, error)
	ListEgress(ctx context.Context, roomName livekit.RoomName, active bool) ([]*livekit.EgressInfo, error)
	UpdateEgress(ctx context.Context, info *livekit.EgressInfo) error
}

// 入口存储接口
// 提供入口的存储、加载、列表等操作
//
//counterfeiter:generate . IngressStore
type IngressStore interface {
	StoreIngress(ctx context.Context, info *livekit.IngressInfo) error
	LoadIngress(ctx context.Context, ingressID string) (*livekit.IngressInfo, error)
	LoadIngressFromStreamKey(ctx context.Context, streamKey string) (*livekit.IngressInfo, error)
	ListIngress(ctx context.Context, roomName livekit.RoomName) ([]*livekit.IngressInfo, error)
	UpdateIngress(ctx context.Context, info *livekit.IngressInfo) error
	UpdateIngressState(ctx context.Context, ingressId string, state *livekit.IngressState) error
	DeleteIngress(ctx context.Context, info *livekit.IngressInfo) error
}

// 房间分配器接口
// 提供房间的自动创建、选择节点、创建房间、验证创建房间等操作
//
//counterfeiter:generate . RoomAllocator
type RoomAllocator interface {
	// 从配置中获取自动创建房间标志
	AutoCreateEnabled(ctx context.Context) bool
	// 选择节点
	SelectRoomNode(ctx context.Context, roomName livekit.RoomName, nodeID livekit.NodeID) error
	// 创建房间
	CreateRoom(ctx context.Context, req *livekit.CreateRoomRequest, isExplicit bool) (*livekit.Room, *livekit.RoomInternal, bool, error)
	// 验证创建房间
	ValidateCreateRoom(ctx context.Context, roomName livekit.RoomName) error
}

// SIP存储接口
// 提供SIP的存储、加载、列表等操作
//
//counterfeiter:generate . SIPStore
type SIPStore interface {
	StoreSIPTrunk(ctx context.Context, info *livekit.SIPTrunkInfo) error
	StoreSIPInboundTrunk(ctx context.Context, info *livekit.SIPInboundTrunkInfo) error
	StoreSIPOutboundTrunk(ctx context.Context, info *livekit.SIPOutboundTrunkInfo) error
	LoadSIPTrunk(ctx context.Context, sipTrunkID string) (*livekit.SIPTrunkInfo, error)
	LoadSIPInboundTrunk(ctx context.Context, sipTrunkID string) (*livekit.SIPInboundTrunkInfo, error)
	LoadSIPOutboundTrunk(ctx context.Context, sipTrunkID string) (*livekit.SIPOutboundTrunkInfo, error)
	ListSIPTrunk(ctx context.Context, opts *livekit.ListSIPTrunkRequest) (*livekit.ListSIPTrunkResponse, error)
	ListSIPInboundTrunk(ctx context.Context, opts *livekit.ListSIPInboundTrunkRequest) (*livekit.ListSIPInboundTrunkResponse, error)
	ListSIPOutboundTrunk(ctx context.Context, opts *livekit.ListSIPOutboundTrunkRequest) (*livekit.ListSIPOutboundTrunkResponse, error)
	DeleteSIPTrunk(ctx context.Context, sipTrunkID string) error

	StoreSIPDispatchRule(ctx context.Context, info *livekit.SIPDispatchRuleInfo) error
	LoadSIPDispatchRule(ctx context.Context, sipDispatchRuleID string) (*livekit.SIPDispatchRuleInfo, error)
	ListSIPDispatchRule(ctx context.Context, opts *livekit.ListSIPDispatchRuleRequest) (*livekit.ListSIPDispatchRuleResponse, error)
	DeleteSIPDispatchRule(ctx context.Context, sipDispatchRuleID string) error
}

// 代理存储接口
// 提供代理的存储、删除、列表等操作
//
//counterfeiter:generate . AgentStore
type AgentStore interface {
	StoreAgentDispatch(ctx context.Context, dispatch *livekit.AgentDispatch) error
	DeleteAgentDispatch(ctx context.Context, dispatch *livekit.AgentDispatch) error
	ListAgentDispatches(ctx context.Context, roomName livekit.RoomName) ([]*livekit.AgentDispatch, error)

	StoreAgentJob(ctx context.Context, job *livekit.Job) error
	DeleteAgentJob(ctx context.Context, job *livekit.Job) error
}
