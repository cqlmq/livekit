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

package prometheus

import (
	"time"

	"github.com/mackerelio/go-osstat/memory"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/rpc"
	"github.com/livekit/protocol/utils/hwstats"
)

const (
	livekitNamespace string = "livekit"

	statsUpdateInterval = time.Second * 10
)

var (
	initialized atomic.Bool

	MessageCounter            *prometheus.CounterVec // 消息计数
	MessageBytes              *prometheus.CounterVec // 消息字节数
	ServiceOperationCounter   *prometheus.CounterVec // 服务操作计数
	TwirpRequestStatusCounter *prometheus.CounterVec // Twirp请求状态计数

	sysPacketsStart              uint32               // 系统级别包计数
	sysDroppedPacketsStart       uint32               // 系统级别丢包计数
	promSysPacketGauge           *prometheus.GaugeVec // 系统级别包计数
	promSysDroppedPacketPctGauge prometheus.Gauge     // 系统级别丢包率

	cpuStats *hwstats.CPUStats // CPU统计
)

// 初始化Prometheus监控
// Initialize Prometheus monitoring
func Init(nodeID string, nodeType livekit.NodeType) error {
	// 如果已经初始化，直接返回
	// 这个用法与sync.Once类似，以后深入理解
	if initialized.Swap(true) {
		return nil
	}

	// 创建Prometheus指标 用于消息计数
	// Create Prometheus metrics for message counting
	MessageCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   livekitNamespace,
			Subsystem:   "node",
			Name:        "messages",
			ConstLabels: prometheus.Labels{"node_id": nodeID, "node_type": nodeType.String()},
		},
		[]string{"type", "status"},
	)

	MessageBytes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   livekitNamespace,
			Subsystem:   "node",
			Name:        "message_bytes",
			ConstLabels: prometheus.Labels{"node_id": nodeID, "node_type": nodeType.String()},
		},
		[]string{"type", "message_type"},
	)

	// 创建Prometheus指标 用于服务操作计数
	// Create Prometheus metrics for service operation counting
	ServiceOperationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   livekitNamespace,
			Subsystem:   "node",
			Name:        "service_operation",
			ConstLabels: prometheus.Labels{"node_id": nodeID, "node_type": nodeType.String()},
		},
		[]string{"type", "status", "error_type"},
	)

	// 创建Prometheus指标 用于Twirp请求状态计数
	// Create Prometheus metrics for Twirp request status counting
	TwirpRequestStatusCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   livekitNamespace,
			Subsystem:   "node",
			Name:        "twirp_request_status",
			ConstLabels: prometheus.Labels{"node_id": nodeID, "node_type": nodeType.String()},
		},
		[]string{"service", "method", "status", "code"},
	)

	// 创建Prometheus指标 用于系统级别包计数
	// Create Prometheus metrics for system level packet count
	promSysPacketGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   livekitNamespace,
			Subsystem:   "node",
			Name:        "packet_total",
			ConstLabels: prometheus.Labels{"node_id": nodeID, "node_type": nodeType.String()},
			Help:        "System level packet count. Count starts at 0 when service is first started.",
		},
		[]string{"type"},
	)

	// 创建Prometheus指标 用于系统级别丢包率
	// Create Prometheus metrics for system level dropped packet percentage
	promSysDroppedPacketPctGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   livekitNamespace,
			Subsystem:   "node",
			Name:        "dropped_packets",
			ConstLabels: prometheus.Labels{"node_id": nodeID, "node_type": nodeType.String()},
			Help:        "System level dropped outgoing packet percentage.",
		},
	)

	prometheus.MustRegister(MessageCounter)
	prometheus.MustRegister(MessageBytes)
	prometheus.MustRegister(ServiceOperationCounter)
	prometheus.MustRegister(TwirpRequestStatusCounter)
	prometheus.MustRegister(promSysPacketGauge)
	prometheus.MustRegister(promSysDroppedPacketPctGauge)

	// 获取系统级别包计数
	sysPacketsStart, sysDroppedPacketsStart, _ = getTCStats()

	initPacketStats(nodeID, nodeType)
	initRoomStats(nodeID, nodeType)
	rpc.InitPSRPCStats(prometheus.Labels{"node_id": nodeID, "node_type": nodeType.String()})
	initQualityStats(nodeID, nodeType)
	initDataPacketStats(nodeID, nodeType)

	var err error
	cpuStats, err = hwstats.NewCPUStats(nil)
	if err != nil {
		return err
	}

	return nil
}

func GetUpdatedNodeStats(prev *livekit.NodeStats, prevAverage *livekit.NodeStats) (*livekit.NodeStats, bool, error) {
	// 获取当前节点的平均负载（1分钟、5分钟或15分钟）
	loadAvg, err := getLoadAvg()
	if err != nil {
		return nil, false, err
	}

	// 获取CPU负载
	var cpuLoad float64
	cpuIdle := cpuStats.GetCPUIdle()
	if cpuIdle > 0 {
		cpuLoad = 1 - (cpuIdle / cpuStats.NumCPU())
	}

	// On MacOS, get "\"vm_stat\": executable file not found in $PATH" although it is in /usr/bin
	// So, do not error out. Use the information if it is available.
	// 获取内存总大小和使用大小
	memTotal := uint64(0)
	memUsed := uint64(0)
	memInfo, _ := memory.Get()
	if memInfo != nil {
		memTotal = memInfo.Total
		memUsed = memInfo.Used
	}

	// do not error out, and use the information if it is available
	// 获取系统级别包计数
	sysPackets, sysDroppedPackets, _ := getTCStats()                                                       // 获取系统级别包计数
	promSysPacketGauge.WithLabelValues("out").Set(float64(sysPackets - sysPacketsStart))                   // 设置系统级别包计数
	promSysPacketGauge.WithLabelValues("dropped").Set(float64(sysDroppedPackets - sysDroppedPacketsStart)) // 设置系统级别丢包计数

	bytesInNow := bytesIn.Load()                                       // 获取字节输入
	bytesOutNow := bytesOut.Load()                                     // 获取字节输出
	packetsInNow := packetsIn.Load()                                   // 获取包输入
	packetsOutNow := packetsOut.Load()                                 // 获取包输出
	nackTotalNow := nackTotal.Load()                                   // 获取nack总数
	retransmitBytesNow := retransmitBytes.Load()                       // 获取重传字节数
	retransmitPacketsNow := retransmitPackets.Load()                   // 获取重传包数
	participantSignalConnectedNow := participantSignalConnected.Load() // 获取参与者信号连接
	participantRTCInitNow := participantRTCInit.Load()                 // 获取参与者RTC初始化
	participantRTConnectedCNow := participantRTCConnected.Load()       // 获取参与者RTC连接
	trackPublishAttemptsNow := trackPublishAttempts.Load()             // 获取发布尝试
	trackPublishSuccessNow := trackPublishSuccess.Load()               // 获取发布成功
	trackSubscribeAttemptsNow := trackSubscribeAttempts.Load()         // 获取订阅尝试
	trackSubscribeSuccessNow := trackSubscribeSuccess.Load()           // 获取订阅成功
	forwardLatencyNow := forwardLatency.Load()                         // 获取转发延迟
	forwardJitterNow := forwardJitter.Load()                           // 获取转发抖动

	updatedAt := time.Now().Unix()               // 获取更新时间
	elapsed := updatedAt - prevAverage.UpdatedAt // 计算时间差
	// include sufficient buffer to be sure a stats update had taken place
	computeAverage := elapsed > int64(statsUpdateInterval.Seconds()+2) // 计算是否需要计算平均值
	if bytesInNow != prevAverage.BytesIn ||
		bytesOutNow != prevAverage.BytesOut ||
		packetsInNow != prevAverage.PacketsIn ||
		packetsOutNow != prevAverage.PacketsOut ||
		retransmitBytesNow != prevAverage.RetransmitBytesOut ||
		retransmitPacketsNow != prevAverage.RetransmitPacketsOut {
		computeAverage = true
	}

	stats := &livekit.NodeStats{
		StartedAt:                        prev.StartedAt,
		UpdatedAt:                        updatedAt,
		NumRooms:                         roomCurrent.Load(),
		NumClients:                       participantCurrent.Load(),
		NumTracksIn:                      trackPublishedCurrent.Load(),
		NumTracksOut:                     trackSubscribedCurrent.Load(),
		NumTrackPublishAttempts:          trackPublishAttemptsNow,
		NumTrackPublishSuccess:           trackPublishSuccessNow,
		NumTrackSubscribeAttempts:        trackSubscribeAttemptsNow,
		NumTrackSubscribeSuccess:         trackSubscribeSuccessNow,
		BytesIn:                          bytesInNow,
		BytesOut:                         bytesOutNow,
		PacketsIn:                        packetsInNow,
		PacketsOut:                       packetsOutNow,
		RetransmitBytesOut:               retransmitBytesNow,
		RetransmitPacketsOut:             retransmitPacketsNow,
		NackTotal:                        nackTotalNow,
		ParticipantSignalConnected:       participantSignalConnectedNow,
		ParticipantRtcInit:               participantRTCInitNow,
		ParticipantRtcConnected:          participantRTConnectedCNow,
		BytesInPerSec:                    prevAverage.BytesInPerSec,
		BytesOutPerSec:                   prevAverage.BytesOutPerSec,
		PacketsInPerSec:                  prevAverage.PacketsInPerSec,
		PacketsOutPerSec:                 prevAverage.PacketsOutPerSec,
		RetransmitBytesOutPerSec:         prevAverage.RetransmitBytesOutPerSec,
		RetransmitPacketsOutPerSec:       prevAverage.RetransmitPacketsOutPerSec,
		NackPerSec:                       prevAverage.NackPerSec,
		ForwardLatency:                   forwardLatencyNow,
		ForwardJitter:                    forwardJitterNow,
		ParticipantSignalConnectedPerSec: prevAverage.ParticipantSignalConnectedPerSec,
		ParticipantRtcInitPerSec:         prevAverage.ParticipantRtcInitPerSec,
		ParticipantRtcConnectedPerSec:    prevAverage.ParticipantRtcConnectedPerSec,
		NumCpus:                          uint32(cpuStats.NumCPU()), // this will round down to the nearest integer
		CpuLoad:                          float32(cpuLoad),
		MemoryTotal:                      memTotal,
		MemoryUsed:                       memUsed,
		LoadAvgLast1Min:                  float32(loadAvg.Loadavg1),
		LoadAvgLast5Min:                  float32(loadAvg.Loadavg5),
		LoadAvgLast15Min:                 float32(loadAvg.Loadavg15),
		SysPacketsOut:                    sysPackets,
		SysPacketsDropped:                sysDroppedPackets,
		TrackPublishAttemptsPerSec:       prevAverage.TrackPublishAttemptsPerSec,
		TrackPublishSuccessPerSec:        prevAverage.TrackPublishSuccessPerSec,
		TrackSubscribeAttemptsPerSec:     prevAverage.TrackSubscribeAttemptsPerSec,
		TrackSubscribeSuccessPerSec:      prevAverage.TrackSubscribeSuccessPerSec,
	}

	// update stats
	if computeAverage {
		stats.BytesInPerSec = perSec(prevAverage.BytesIn, bytesInNow, elapsed)
		stats.BytesOutPerSec = perSec(prevAverage.BytesOut, bytesOutNow, elapsed)
		stats.PacketsInPerSec = perSec(prevAverage.PacketsIn, packetsInNow, elapsed)
		stats.PacketsOutPerSec = perSec(prevAverage.PacketsOut, packetsOutNow, elapsed)
		stats.RetransmitBytesOutPerSec = perSec(prevAverage.RetransmitBytesOut, retransmitBytesNow, elapsed)
		stats.RetransmitPacketsOutPerSec = perSec(prevAverage.RetransmitPacketsOut, retransmitPacketsNow, elapsed)
		stats.NackPerSec = perSec(prevAverage.NackTotal, nackTotalNow, elapsed)
		stats.ParticipantSignalConnectedPerSec = perSec(prevAverage.ParticipantSignalConnected, participantSignalConnectedNow, elapsed)
		stats.ParticipantRtcInitPerSec = perSec(prevAverage.ParticipantRtcInit, participantRTCInitNow, elapsed)
		stats.ParticipantRtcConnectedPerSec = perSec(prevAverage.ParticipantRtcConnected, participantRTConnectedCNow, elapsed)
		stats.SysPacketsOutPerSec = perSec(uint64(prevAverage.SysPacketsOut), uint64(sysPackets), elapsed)
		stats.SysPacketsDroppedPerSec = perSec(uint64(prevAverage.SysPacketsDropped), uint64(sysDroppedPackets), elapsed)
		stats.TrackPublishAttemptsPerSec = perSec(uint64(prevAverage.NumTrackPublishAttempts), uint64(trackPublishAttemptsNow), elapsed)
		stats.TrackPublishSuccessPerSec = perSec(uint64(prevAverage.NumTrackPublishSuccess), uint64(trackPublishSuccessNow), elapsed)
		stats.TrackSubscribeAttemptsPerSec = perSec(uint64(prevAverage.NumTrackSubscribeAttempts), uint64(trackSubscribeAttemptsNow), elapsed)
		stats.TrackSubscribeSuccessPerSec = perSec(uint64(prevAverage.NumTrackSubscribeSuccess), uint64(trackSubscribeSuccessNow), elapsed)

		packetTotal := stats.SysPacketsOutPerSec + stats.SysPacketsDroppedPerSec
		if packetTotal == 0 {
			stats.SysPacketsDroppedPctPerSec = 0
		} else {
			stats.SysPacketsDroppedPctPerSec = stats.SysPacketsDroppedPerSec / packetTotal
		}
		promSysDroppedPacketPctGauge.Set(float64(stats.SysPacketsDroppedPctPerSec))
	}

	return stats, computeAverage, nil
}

func perSec(prev, curr uint64, secs int64) float32 {
	return float32(curr-prev) / float32(secs)
}
