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
	"bytes"
	"context"
	"runtime/pprof"
	"time"

	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/rpc"

	"github.com/livekit/livekit-server/pkg/routing/selector"
)

const (
	// expire participant mappings after a day
	participantMappingTTL = 24 * time.Hour  // 参与者映射过期时间
	statsUpdateInterval   = 2 * time.Second // 统计更新间隔
	statsMaxDelaySeconds  = float64(30)     // 统计最大延迟秒数

	// hash of node_id => Node proto
	NodesKey = "nodes"

	// hash of room_name => node_id
	NodeRoomKey = "room_node_map"
)

var _ Router = (*RedisRouter)(nil)

// RedisRouter uses Redis pub/sub to route signaling messages across different nodes
// It relies on the RTC node to be the primary driver of the participant connection.
// Because
// RedisRouter 使用 Redis 发布/订阅来跨不同节点路由信令消息
// 它依赖于 RTC 节点成为参与者连接的主要驱动程序。
// 因为
type RedisRouter struct {
	*LocalRouter

	rc        redis.UniversalClient // redis客户端
	kps       rpc.KeepalivePubSub   // 保持活跃的发布订阅
	ctx       context.Context       // 上下文
	isStarted atomic.Bool           // 是否启动

	cancel func() // 取消函数
}

func NewRedisRouter(lr *LocalRouter, rc redis.UniversalClient, kps rpc.KeepalivePubSub) *RedisRouter {
	rr := &RedisRouter{
		LocalRouter: lr,
		rc:          rc,
		kps:         kps,
	}
	rr.ctx, rr.cancel = context.WithCancel(context.Background())
	return rr
}

// 注册节点
// 会定时更新节点状态，类似心跳的机制
func (r *RedisRouter) RegisterNode() error {
	// data, err := proto.Marshal(r.currentNode.Clone())
	// if err != nil {
	// 	return err
	// }
	// nd := r.currentNode.Clone()
	// buf, _ := json.MarshalIndent(nd, "", "  ")
	// fmt.Println("RegisterNode currentNode:", string(buf))

	// 直接转换成proto的[]byte，减少一次克隆
	data, err := r.currentNode.ToProtoBytes()
	if err != nil {
		return err
	}
	if err := r.rc.HSet(r.ctx, NodesKey, string(r.currentNode.NodeID()), data).Err(); err != nil {
		return errors.Wrap(err, "could not register node")
	}
	return nil
}

func (r *RedisRouter) UnregisterNode() error {
	// could be called after Stop(), so we'd want to use an unrelated context
	return r.rc.HDel(context.Background(), NodesKey, string(r.currentNode.NodeID())).Err()
}

func (r *RedisRouter) RemoveDeadNodes() error {
	nodes, err := r.ListNodes()
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if !selector.IsAvailable(n) {
			if err := r.rc.HDel(context.Background(), NodesKey, n.Id).Err(); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetNodeForRoom finds the node where the room is hosted at
func (r *RedisRouter) GetNodeForRoom(_ context.Context, roomName livekit.RoomName) (*livekit.Node, error) {
	nodeID, err := r.rc.HGet(r.ctx, NodeRoomKey, string(roomName)).Result()
	if err == redis.Nil {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "could not get node for room")
	}

	return r.GetNode(livekit.NodeID(nodeID))
}

func (r *RedisRouter) SetNodeForRoom(_ context.Context, roomName livekit.RoomName, nodeID livekit.NodeID) error {
	return r.rc.HSet(r.ctx, NodeRoomKey, string(roomName), string(nodeID)).Err()
}

func (r *RedisRouter) ClearRoomState(_ context.Context, roomName livekit.RoomName) error {
	if err := r.rc.HDel(context.Background(), NodeRoomKey, string(roomName)).Err(); err != nil {
		return errors.Wrap(err, "could not clear room state")
	}
	return nil
}

func (r *RedisRouter) GetNode(nodeID livekit.NodeID) (*livekit.Node, error) {
	data, err := r.rc.HGet(r.ctx, NodesKey, string(nodeID)).Result()
	if err == redis.Nil {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	node := &livekit.Node{}
	if err = proto.Unmarshal([]byte(data), node); err != nil {
		return nil, err
	}
	return node, nil
}

func (r *RedisRouter) ListNodes() ([]*livekit.Node, error) {
	items, err := r.rc.HVals(r.ctx, NodesKey).Result()
	if err != nil {
		return nil, errors.Wrap(err, "could not list nodes")
	}
	nodes := make([]*livekit.Node, 0, len(items))
	for _, item := range items {
		node := &livekit.Node{}
		if err := proto.Unmarshal([]byte(item), node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (r *RedisRouter) CreateRoom(ctx context.Context, req *livekit.CreateRoomRequest) (res *livekit.Room, err error) {
	rtcNode, err := r.GetNodeForRoom(ctx, livekit.RoomName(req.Name))
	if err != nil {
		return
	}

	return r.CreateRoomWithNodeID(ctx, req, livekit.NodeID(rtcNode.Id))
}

// StartParticipantSignal signal connection sets up paths to the RTC node, and starts to route messages to that message queue
// 信令连接设置到 RTC 节点的路径，并开始将消息路由到该消息队列
func (r *RedisRouter) StartParticipantSignal(ctx context.Context, roomName livekit.RoomName, pi ParticipantInit) (res StartParticipantSignalResults, err error) {
	// 根据房间名获取 RTC 节点
	rtcNode, err := r.GetNodeForRoom(ctx, roomName)
	if err != nil {
		return
	}

	// 开始参与者信令
	return r.StartParticipantSignalWithNodeID(ctx, roomName, pi, livekit.NodeID(rtcNode.Id))
}

func (r *RedisRouter) Start() error {
	if r.isStarted.Swap(true) {
		return nil
	}

	workerStarted := make(chan error)
	go r.statsWorker()
	go r.keepaliveWorker(workerStarted)

	// wait until worker is running
	return <-workerStarted
}

func (r *RedisRouter) Drain() {
	r.currentNode.SetState(livekit.NodeState_SHUTTING_DOWN)
	if err := r.RegisterNode(); err != nil {
		logger.Errorw("failed to mark as draining", err, "nodeID", r.currentNode.NodeID())
	}
}

func (r *RedisRouter) Stop() {
	if !r.isStarted.Swap(false) {
		return
	}
	logger.Debugw("stopping RedisRouter")
	_ = r.UnregisterNode()
	r.cancel()
}

// update node stats and cleanup
func (r *RedisRouter) statsWorker() {
	goroutineDumped := false
	for r.ctx.Err() == nil {
		// update periodically
		select {
		case <-time.After(statsUpdateInterval):
			// 发布心跳(秒)
			r.kps.PublishPing(r.ctx, r.currentNode.NodeID(), &rpc.KeepalivePing{Timestamp: time.Now().Unix()})

			// 检查延迟
			delaySeconds := r.currentNode.SecondsSinceNodeStatsUpdate()
			if delaySeconds > statsMaxDelaySeconds {
				if !goroutineDumped {
					goroutineDumped = true // 设置为 true 在没有恢复之前不会重复打印
					buf := bytes.NewBuffer(nil)
					_ = pprof.Lookup("goroutine").WriteTo(buf, 2)
					logger.Errorw("status update delayed, possible deadlock", nil,
						"delay", delaySeconds,
						"goroutines", buf.String())
				}
			} else {
				goroutineDumped = false
			}
		case <-r.ctx.Done():
			return
		}
	}
}

func (r *RedisRouter) keepaliveWorker(startedChan chan error) {
	pings, err := r.kps.SubscribePing(r.ctx, r.currentNode.NodeID())
	if err != nil {
		startedChan <- err
		return
	}
	close(startedChan)

	for ping := range pings.Channel() {
		// 打印订阅的消息
		// fmt.Println("keepaliveWorker SubscribePing==================", ping)

		// 检查心跳是否过期
		if time.Since(time.Unix(ping.Timestamp, 0)) > statsUpdateInterval {
			logger.Infow("keep alive too old, skipping", "timestamp", ping.Timestamp)
			continue
		}

		// 更新节点统计信息
		if !r.currentNode.UpdateNodeStats() {
			continue
		}

		// TODO: check stats against config.Limit values // 检查统计信息是否符合配置的限制值

		// 注册节点
		if err := r.RegisterNode(); err != nil {
			logger.Errorw("could not update node", err)
		}
	}
}
