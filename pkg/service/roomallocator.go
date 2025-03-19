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
	"errors"
	"time"

	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/utils"
	"github.com/livekit/protocol/utils/guid"
	"github.com/livekit/psrpc"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/pkg/routing/selector"
)

// StandardRoomAllocator 标准房间分配器
type StandardRoomAllocator struct {
	config    *config.Config        // 配置
	router    routing.Router        // 路由器
	selector  selector.NodeSelector // 节点选择器
	roomStore ObjectStore           // 房间存储
}

// NewRoomAllocator 创建房间分配器
// wire_gen.go中调用初始化一个StandardRoomAllocator
func NewRoomAllocator(conf *config.Config, router routing.Router, rs ObjectStore) (RoomAllocator, error) {
	ns, err := selector.CreateNodeSelector(conf)
	if err != nil {
		return nil, err
	}

	return &StandardRoomAllocator{
		config:    conf,
		router:    router,
		selector:  ns,
		roomStore: rs,
	}, nil
}

// AutoCreateEnabled 从配置中获取自动创建房间标志
// 比较简单，RoomAllocator接口需要实现这个方法
func (r *StandardRoomAllocator) AutoCreateEnabled(context.Context) bool {
	return r.config.Room.AutoCreate
}

// CreateRoom creates a new room from a request and allocates it to a node to handle
// it'll also monitor its state, and cleans it up when appropriate
// 创建房间，并分配给一个节点来处理
// 它会监控房间的状态，并在适当的时候清理它
func (r *StandardRoomAllocator) CreateRoom(ctx context.Context, req *livekit.CreateRoomRequest, isExplicit bool) (*livekit.Room, *livekit.RoomInternal, bool, error) {
	token, err := r.roomStore.LockRoom(ctx, livekit.RoomName(req.Name), 5*time.Second)
	if err != nil {
		return nil, nil, false, err
	}
	defer func() {
		_ = r.roomStore.UnlockRoom(ctx, livekit.RoomName(req.Name), token)
	}()

	// find existing room and update it
	var created bool
	rm, internal, err := r.roomStore.LoadRoom(ctx, livekit.RoomName(req.Name), true)
	if errors.Is(err, ErrRoomNotFound) {
		created = true
		now := time.Now()
		rm = &livekit.Room{
			Sid:            guid.New(utils.RoomPrefix),
			Name:           req.Name,
			CreationTime:   now.Unix(),
			CreationTimeMs: now.UnixMilli(),
			TurnPassword:   utils.RandomSecret(),
		}
		internal = &livekit.RoomInternal{}
		applyDefaultRoomConfig(rm, internal, &r.config.Room)
	} else if err != nil {
		return nil, nil, false, err
	}

	req, err = r.applyNamedRoomConfiguration(req)
	if err != nil {
		return nil, nil, false, err
	}

	if req.EmptyTimeout > 0 {
		rm.EmptyTimeout = req.EmptyTimeout
	}
	if req.DepartureTimeout > 0 {
		rm.DepartureTimeout = req.DepartureTimeout
	}
	if req.MaxParticipants > 0 {
		rm.MaxParticipants = req.MaxParticipants
	}
	if req.Metadata != "" {
		rm.Metadata = req.Metadata
	}
	if req.Egress != nil {
		if req.Egress.Participant != nil {
			internal.ParticipantEgress = req.Egress.Participant
		}
		if req.Egress.Tracks != nil {
			internal.TrackEgress = req.Egress.Tracks
		}
	}
	if req.Agents != nil {
		internal.AgentDispatches = req.Agents
	}
	if req.MinPlayoutDelay > 0 || req.MaxPlayoutDelay > 0 {
		internal.PlayoutDelay = &livekit.PlayoutDelay{
			Enabled: true,
			Min:     req.MinPlayoutDelay,
			Max:     req.MaxPlayoutDelay,
		}
	}
	if req.SyncStreams {
		internal.SyncStreams = true
	}

	if err = r.roomStore.StoreRoom(ctx, rm, internal); err != nil {
		return nil, nil, false, err
	}

	return rm, internal, created, nil
}

// 选择房间节点
func (r *StandardRoomAllocator) SelectRoomNode(ctx context.Context, roomName livekit.RoomName, nodeID livekit.NodeID) error {
	// check if room already assigned
	// 检查房间是否已经分配了节点，如果不存在并且错误类型不是routing.ErrNotFound，则返回错误
	existing, err := r.router.GetNodeForRoom(ctx, roomName)
	if !errors.Is(err, routing.ErrNotFound) && err != nil {
		return err
	}

	// if already assigned and still available, keep it on that node
	// 如果房间已经分配了节点，并且节点仍然可用，则保持在该节点上
	if err == nil && selector.IsAvailable(existing) {
		// if node hosting the room is full, deny entry
		// 如果节点承载的房间已满，则拒绝进入
		if selector.LimitsReached(r.config.Limit, existing.Stats) {
			return routing.ErrNodeLimitReached
		}

		return nil
	}

	// select a new node
	if nodeID == "" {
		nodes, err := r.router.ListNodes()
		if err != nil {
			return err
		}

		node, err := r.selector.SelectNode(nodes)
		if err != nil {
			return err
		}

		nodeID = livekit.NodeID(node.Id)
	}

	logger.Infow("selected node for room", "room", roomName, "selectedNodeID", nodeID)
	err = r.router.SetNodeForRoom(ctx, roomName, nodeID)
	if err != nil {
		return err
	}

	return nil
}

func (r *StandardRoomAllocator) ValidateCreateRoom(ctx context.Context, roomName livekit.RoomName) error {
	// when auto create is disabled, we'll check to ensure it's already created
	if !r.config.Room.AutoCreate {
		_, _, err := r.roomStore.LoadRoom(ctx, roomName, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func applyDefaultRoomConfig(room *livekit.Room, internal *livekit.RoomInternal, conf *config.RoomConfig) {
	room.EmptyTimeout = conf.EmptyTimeout
	room.DepartureTimeout = conf.DepartureTimeout
	room.MaxParticipants = conf.MaxParticipants
	for _, codec := range conf.EnabledCodecs {
		room.EnabledCodecs = append(room.EnabledCodecs, &livekit.Codec{
			Mime:     codec.Mime,
			FmtpLine: codec.FmtpLine,
		})
	}
	internal.PlayoutDelay = &livekit.PlayoutDelay{
		Enabled: conf.PlayoutDelay.Enabled,
		Min:     uint32(conf.PlayoutDelay.Min),
		Max:     uint32(conf.PlayoutDelay.Max),
	}
	internal.SyncStreams = conf.SyncStreams
}

func (r *StandardRoomAllocator) applyNamedRoomConfiguration(req *livekit.CreateRoomRequest) (*livekit.CreateRoomRequest, error) {
	if req.RoomPreset == "" {
		return req, nil
	}

	conf, ok := r.config.Room.RoomConfigurations[req.RoomPreset]
	if !ok {
		return req, psrpc.NewErrorf(psrpc.InvalidArgument, "unknown room confguration in create room request")
	}

	clone := utils.CloneProto(req)

	// Request overwrites conf
	if clone.EmptyTimeout == 0 {
		clone.EmptyTimeout = conf.EmptyTimeout
	}
	if clone.DepartureTimeout == 0 {
		clone.DepartureTimeout = req.DepartureTimeout
	}
	if clone.MaxParticipants == 0 {
		clone.MaxParticipants = conf.MaxParticipants
	}
	if clone.Egress == nil {
		clone.Egress = utils.CloneProto(conf.Egress)
	}
	if clone.Agents == nil {
		clone.Agents = make([]*livekit.RoomAgentDispatch, 0, len(conf.Agents))
		for _, agent := range conf.Agents {
			clone.Agents = append(clone.Agents, utils.CloneProto(agent))
		}
	}
	if clone.MinPlayoutDelay == 0 {
		clone.MinPlayoutDelay = conf.MinPlayoutDelay
	}
	if clone.MaxPlayoutDelay == 0 {
		clone.MaxPlayoutDelay = conf.MaxPlayoutDelay
	}
	if !clone.SyncStreams {
		clone.SyncStreams = conf.SyncStreams
	}

	return clone, nil
}
