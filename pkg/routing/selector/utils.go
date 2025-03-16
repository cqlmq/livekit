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

package selector

import (
	"sort"
	"time"

	"github.com/thoas/go-funk"

	"github.com/livekit/protocol/livekit"

	"github.com/livekit/livekit-server/pkg/config"
)

const AvailableSeconds = 5

// checks if a node has been updated recently to be considered for selection
// 判断节点是否在可用
func IsAvailable(node *livekit.Node) bool {
	if node.Stats == nil {
		// available till stats are available
		// 如果节点没有统计信息，也认为节点可用
		return true
	}
	// 检查节点统计信息更新时间是否在可用时间内
	delta := time.Now().Unix() - node.Stats.UpdatedAt
	return int(delta) < AvailableSeconds
}

// 过滤出可用节点
func GetAvailableNodes(nodes []*livekit.Node) []*livekit.Node {
	return funk.Filter(nodes, func(node *livekit.Node) bool {
		return IsAvailable(node) && node.State == livekit.NodeState_SERVING
	}).([]*livekit.Node)
}

// 获取节点负载（使用1分钟负载）
func GetNodeSysload(node *livekit.Node) float32 {
	stats := node.Stats
	numCpus := stats.NumCpus
	if numCpus == 0 {
		numCpus = 1
	}
	return stats.LoadAvgLast1Min / float32(numCpus)
}

// TODO: check remote node configured limit, instead of this node's config
// 检查节点是否达到限制
func LimitsReached(limitConfig config.LimitConfig, nodeStats *livekit.NodeStats) bool {
	if nodeStats == nil {
		return false
	}

	if limitConfig.NumTracks > 0 && limitConfig.NumTracks <= nodeStats.NumTracksIn+nodeStats.NumTracksOut {
		return true
	}
	if limitConfig.BytesPerSec > 0 && limitConfig.BytesPerSec <= nodeStats.BytesInPerSec+nodeStats.BytesOutPerSec {
		return true
	}

	return false
}

// 使用指定的方法选择节点
func SelectSortedNode(nodes []*livekit.Node, sortBy string) (*livekit.Node, error) {
	if sortBy == "" {
		return nil, ErrSortByNotSet
	}

	// Return a node based on what it should be sorted by for priority
	switch sortBy {
	case "random":
		// 随机选择
		idx := funk.RandomInt(0, len(nodes))
		return nodes[idx], nil
	case "sysload":
		// 根据整体负载选择
		sort.Slice(nodes, func(i, j int) bool {
			return GetNodeSysload(nodes[i]) < GetNodeSysload(nodes[j])
		})
		return nodes[0], nil
	case "cpuload":
		// 根据CPU负载选择
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Stats.CpuLoad < nodes[j].Stats.CpuLoad
		})
		return nodes[0], nil
	case "rooms":
		// 根据房间数选择
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Stats.NumRooms < nodes[j].Stats.NumRooms
		})
		return nodes[0], nil
	case "clients":
		// 根据客户端数选择
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Stats.NumClients < nodes[j].Stats.NumClients
		})
		return nodes[0], nil
	case "tracks":
		// 根据音视频流数选择
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Stats.NumTracksIn+nodes[i].Stats.NumTracksOut < nodes[j].Stats.NumTracksIn+nodes[j].Stats.NumTracksOut
		})
		return nodes[0], nil
	case "bytespersec":
		// 根据流量选择
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Stats.BytesInPerSec+nodes[i].Stats.BytesOutPerSec < nodes[j].Stats.BytesInPerSec+nodes[j].Stats.BytesOutPerSec
		})
		return nodes[0], nil
	default:
		return nil, ErrSortByUnknown
	}
}
