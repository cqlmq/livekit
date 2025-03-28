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
	"encoding/json"
	"runtime"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/utils"
	"github.com/livekit/protocol/utils/guid"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/telemetry/prometheus"
)

// LocalNode 本地节点接口
type LocalNode interface {
	Clone() *livekit.Node                 // 克隆一个节点
	SetNodeID(nodeID livekit.NodeID)      // 设置节点ID
	NodeID() livekit.NodeID               // 获取节点ID
	NodeType() livekit.NodeType           // 获取节点类型
	NodeIP() string                       // 获取节点IP
	Region() string                       // 获取节点区域
	SetState(state livekit.NodeState)     // 设置节点状态
	SetStats(stats *livekit.NodeStats)    // 设置节点统计信息
	UpdateNodeStats() bool                // 更新节点统计信息
	SecondsSinceNodeStatsUpdate() float64 // 获取节点统计信息更新时间
}

// LocalNodeImpl 本地节点实现
type LocalNodeImpl struct {
	lock sync.RWMutex
	node *livekit.Node

	// previous stats for computing averages
	// 前一个统计信息，用于计算平均值
	prevStats *livekit.NodeStats
}

// NewLocalNode 创建一个本地节点
// 没有配置也行，但是需要设置NodeIP及Region （以后看是否有这样的场景）
func NewLocalNode(conf *config.Config) (*LocalNodeImpl, error) {
	if conf != nil && conf.RTC.NodeIP == "" {
		return nil, ErrIPNotSet
	}

	// 生成一个唯一的节点ID b57编码 所以采用了自定义的guid.New
	nodeID := guid.New(utils.NodePrefix)
	l := &LocalNodeImpl{
		node: &livekit.Node{
			Id:      nodeID,
			NumCpus: uint32(runtime.NumCPU()),
			State:   livekit.NodeState_SERVING,
			Stats: &livekit.NodeStats{
				StartedAt: time.Now().Unix(),
				UpdatedAt: time.Now().Unix(),
			},
		},
	}
	if conf != nil {
		l.node.Ip = conf.RTC.NodeIP
		l.node.Region = conf.Region
	}
	return l, nil
}

// 从livekit.Node 新创创建 LocalNodeImpl
func NewLocalNodeFromNodeProto(node *livekit.Node) (*LocalNodeImpl, error) {
	return &LocalNodeImpl{node: utils.CloneProto(node)}, nil
}

func (l *LocalNodeImpl) Clone() *livekit.Node {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return utils.CloneProto(l.node)
}

// for testing only
func (l *LocalNodeImpl) SetNodeID(nodeID livekit.NodeID) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.node.Id = string(nodeID)
}

func (l *LocalNodeImpl) NodeID() livekit.NodeID {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return livekit.NodeID(l.node.Id)
}

func (l *LocalNodeImpl) NodeType() livekit.NodeType {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.node.Type
}

func (l *LocalNodeImpl) NodeIP() string {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.node.Ip
}

func (l *LocalNodeImpl) Region() string {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.node.Region
}

func (l *LocalNodeImpl) SetState(state livekit.NodeState) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.node.State = state
}

// for testing only
func (l *LocalNodeImpl) SetStats(stats *livekit.NodeStats) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.node.Stats = utils.CloneProto(stats)
}

// 更新节点统计信息
func (l *LocalNodeImpl) UpdateNodeStats() bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.prevStats == nil {
		l.prevStats = l.node.Stats
	}
	updated, computedAvg, err := prometheus.GetUpdatedNodeStats(l.node.Stats, l.prevStats)
	if err != nil {
		logger.Errorw("could not update node stats", err)
		return false
	}
	l.node.Stats = updated
	if computedAvg {
		l.prevStats = updated
	}
	return true
}

// 获取节点统计信息更新时间过去多少秒
func (l *LocalNodeImpl) SecondsSinceNodeStatsUpdate() float64 {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return time.Since(time.Unix(l.node.Stats.UpdatedAt, 0)).Seconds()
}

// 序列化
func (l *LocalNodeImpl) Marshal() ([]byte, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return json.MarshalIndent(l.node, "", "  ")
}
