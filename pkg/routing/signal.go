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
	"errors"
	"sync"
	"time"

	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/telemetry/prometheus"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/rpc"
	"github.com/livekit/protocol/utils/guid"
	"github.com/livekit/psrpc"
	"github.com/livekit/psrpc/pkg/middleware"
)

var ErrSignalWriteFailed = errors.New("signal write failed")
var ErrSignalMessageDropped = errors.New("signal message dropped")

/*
代码讲解：
1. 信令消息处理
   - signalRequestMessageWriter：处理请求消息的写入
   - signalResponseMessageReader：处理响应消息的读取
   - signalMessageSink：实现消息的异步写入和队列管理

2. 消息传输控制
   - 实现了消息序列号管理
   - 支持消息重试机制
   - 包含超时和错误处理

关键特性：
   - 异步消息处理：使用 goroutine 进行异步消息写入
   - 消息队列管理：实现了消息缓冲和队列管理
   - 错误处理：包含完整的错误处理和重试机制
   - 连接管理：支持连接计数和状态管理
   - 监控指标：集成了 Prometheus 指标收集

重要方法：
   - StartParticipantSignal：启动参与者信令连接
   - WriteMessage：写入消息到队列
   - Close：关闭信令连接
   - CopySignalStreamToMessageChannel：复制信号流到消息通道

安全特性：
   - 使用互斥锁保证并发安全
   - 实现了消息序列号验证
   - 支持优雅关闭和资源清理
*/

// SignalClient 信令客户端接口
//
//counterfeiter:generate . SignalClient
type SignalClient interface {
	// 获取活跃的信令连接数
	ActiveCount() int
	// 启动参与者信令
	StartParticipantSignal(ctx context.Context, roomName livekit.RoomName, pi ParticipantInit, nodeID livekit.NodeID) (connectionID livekit.ConnectionID, reqSink MessageSink, resSource MessageSource, err error)
}

// signalClient 信令客户端结构与实现
type signalClient struct {
	nodeID livekit.NodeID           // 节点ID
	config config.SignalRelayConfig // 信令配置
	client rpc.TypedSignalClient    // 信令客户端
	active atomic.Int32             // 活跃的信令连接数
}

// NewSignalClient 创建信令客户端
func NewSignalClient(nodeID livekit.NodeID, bus psrpc.MessageBus, config config.SignalRelayConfig) (SignalClient, error) {
	c, err := rpc.NewTypedSignalClient(
		nodeID,
		bus,
		middleware.WithClientMetrics(rpc.PSRPCMetricsObserver{}), // 添加客户端指标
		psrpc.WithClientChannelSize(config.StreamBufferSize),     // 设置流缓冲区大小
	)
	if err != nil {
		return nil, err
	}

	return &signalClient{
		nodeID: nodeID,
		config: config,
		client: c,
	}, nil
}

// 获取活跃的信号连接数
func (r *signalClient) ActiveCount() int {
	return int(r.active.Load())
}

// StartParticipantSignal 开始参与者信令
func (r *signalClient) StartParticipantSignal(
	ctx context.Context,
	roomName livekit.RoomName,
	pi ParticipantInit,
	nodeID livekit.NodeID,
) (
	connectionID livekit.ConnectionID,
	reqSink MessageSink,
	resSource MessageSource,
	err error,
) {
	connectionID = livekit.ConnectionID(guid.New("CO_"))
	ss, err := pi.ToStartSession(roomName, connectionID)
	if err != nil {
		return
	}

	log := logger.GetLogger().WithValues(
		"room", roomName,
		"reqNodeID", nodeID,
		"participant", pi.Identity,
		"connID", connectionID,
		"participantInit", pi,
		"startSession", logger.Proto(ss),
	)

	log.Debugw("starting signal connection")

	stream, err := r.client.RelaySignal(ctx, nodeID)
	if err != nil {
		prometheus.MessageCounter.WithLabelValues("signal", "failure").Add(1)
		return
	}

	// 发送开始会话请求
	err = stream.Send(&rpc.RelaySignalRequest{StartSession: ss})
	if err != nil {
		stream.Close(err)
		prometheus.MessageCounter.WithLabelValues("signal", "failure").Add(1)
		return
	}

	params := SignalSinkParams[*rpc.RelaySignalRequest, *rpc.RelaySignalResponse]{
		Logger:         log,
		Stream:         stream,
		Config:         r.config,
		Writer:         signalRequestMessageWriter{},
		CloseOnFailure: true,
		BlockOnClose:   true,
		ConnectionID:   connectionID,
	}
	sink := NewSignalMessageSink(params)
	resChan := NewDefaultMessageChannel(connectionID)

	go func() {
		r.active.Inc()       // 增加活跃的信号连接数
		defer r.active.Dec() // 减少活跃的信号连接数

		// 复制信号流到消息通道（一直复制，直到关闭）
		err := CopySignalStreamToMessageChannel(
			stream,
			resChan,
			signalResponseMessageReader{},
			r.config,
		)
		log.Debugw("signal stream closed", "error", err)

		resChan.Close()
	}()

	return connectionID, sink, resChan, nil
}

// 信令消息写入器定义与实现
type signalRequestMessageWriter struct{}

func (e signalRequestMessageWriter) Write(seq uint64, close bool, msgs []proto.Message) *rpc.RelaySignalRequest {
	r := &rpc.RelaySignalRequest{
		Seq:      seq,
		Requests: make([]*livekit.SignalRequest, 0, len(msgs)),
		Close:    close,
	}
	for _, m := range msgs {
		r.Requests = append(r.Requests, m.(*livekit.SignalRequest))
	}
	return r
}

// 信令消息读取器定义与实现
type signalResponseMessageReader struct{}

func (e signalResponseMessageReader) Read(rm *rpc.RelaySignalResponse) ([]proto.Message, error) {
	msgs := make([]proto.Message, 0, len(rm.Responses))
	for _, m := range rm.Responses {
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// 信号消息接口
type RelaySignalMessage interface {
	proto.Message   // 消息
	GetSeq() uint64 // 获取消息序列号
	GetClose() bool // 获取关闭标志
}

// 信号消息写入器接口
type SignalMessageWriter[SendType RelaySignalMessage] interface {
	Write(seq uint64, close bool, msgs []proto.Message) SendType
}

// 信号消息读取器接口
type SignalMessageReader[RecvType RelaySignalMessage] interface {
	Read(msg RecvType) ([]proto.Message, error)
}

// 复制信号流到消息通道
func CopySignalStreamToMessageChannel[SendType, RecvType RelaySignalMessage](
	stream psrpc.Stream[SendType, RecvType],
	ch *MessageChannel,
	reader SignalMessageReader[RecvType],
	config config.SignalRelayConfig,
) error {
	r := &signalMessageReader[SendType, RecvType]{
		reader: reader,
		config: config,
	}
	for msg := range stream.Channel() {
		resArray, err := r.Read(msg)
		if err != nil {
			prometheus.MessageCounter.WithLabelValues("signal", "failure").Add(1)
			return err
		}

		for _, res := range resArray {
			if err = ch.WriteMessage(res); err != nil {
				prometheus.MessageCounter.WithLabelValues("signal", "failure").Add(1)
				return err
			}
			prometheus.MessageCounter.WithLabelValues("signal", "success").Add(1)
		}

		if msg.GetClose() {
			return stream.Close(nil)
		}
	}
	return stream.Err()
}

type signalMessageReader[SendType, RecvType RelaySignalMessage] struct {
	seq    uint64
	reader SignalMessageReader[RecvType]
	config config.SignalRelayConfig
}

func (r *signalMessageReader[SendType, RecvType]) Read(msg RecvType) ([]proto.Message, error) {
	res, err := r.reader.Read(msg)
	if err != nil {
		return nil, err
	}

	// 检查消息序列号 如果大于预期，则丢弃多余的消息
	if r.seq < msg.GetSeq() {
		return nil, ErrSignalMessageDropped
	}

	// 如果消息序列号小于当前序列号，则丢弃多余的消息
	if r.seq > msg.GetSeq() {
		n := int(r.seq - msg.GetSeq())
		if n > len(res) {
			n = len(res)
		}
		res = res[n:]
	}
	r.seq += uint64(len(res))

	return res, nil
}

// 信号消息写入器参数
type SignalSinkParams[SendType, RecvType RelaySignalMessage] struct {
	Stream         psrpc.Stream[SendType, RecvType]
	Logger         logger.Logger
	Config         config.SignalRelayConfig
	Writer         SignalMessageWriter[SendType]
	CloseOnFailure bool
	BlockOnClose   bool
	ConnectionID   livekit.ConnectionID
}

func NewSignalMessageSink[SendType, RecvType RelaySignalMessage](params SignalSinkParams[SendType, RecvType]) MessageSink {
	return &signalMessageSink[SendType, RecvType]{
		SignalSinkParams: params,
	}
}

// 信令消息写入器定义与实现
// 实现消息的异步写入和队列管理
type signalMessageSink[SendType, RecvType RelaySignalMessage] struct {
	SignalSinkParams[SendType, RecvType]

	mu       sync.Mutex      // 互斥锁
	seq      uint64          // 消息序列号
	queue    []proto.Message // 消息队列
	writing  bool            // 写入标志
	draining bool            // 关闭标志
}

func (s *signalMessageSink[SendType, RecvType]) Close() {
	s.mu.Lock()
	s.draining = true // 设置关闭标志
	if !s.writing {
		s.writing = true
		go s.write()
	}
	s.mu.Unlock()

	// conditionally block while closing to wait for outgoing messages to drain
	//
	// on media the signal sink shares a goroutine with other signal connection
	// attempts from the same participant so blocking delays establishing new
	// sessions during reconnect.
	//
	// on controller closing without waiting for the outstanding messages to
	// drain causes leave messages to be dropped from the write queue. when
	// this happens other participants in the room aren't notified about the
	// departure until the participant times out.
	if s.BlockOnClose {
		<-s.Stream.Context().Done()
	}
}

func (s *signalMessageSink[SendType, RecvType]) IsClosed() bool {
	return s.Stream.Err() != nil
}

// 写入消息(线程方式运行，通过s.writing控制每个对齐同时只启动动一个线程写入)
func (s *signalMessageSink[SendType, RecvType]) write() {
	interval := s.Config.MinRetryInterval             // 最小重试间隔
	deadline := time.Now().Add(s.Config.RetryTimeout) // 重试超时时间
	var err error                                     // 错误

	s.mu.Lock()
	for {
		close := s.draining // 关闭标志
		if (!close && len(s.queue) == 0) || s.IsClosed() {
			break
		}
		msg, n := s.Writer.Write(s.seq, close, s.queue), len(s.queue)
		s.mu.Unlock()

		err = s.Stream.Send(msg, psrpc.WithTimeout(interval))
		if err != nil {
			if time.Now().After(deadline) {
				s.Logger.Warnw("could not send signal message", err)

				s.mu.Lock()
				s.seq += uint64(len(s.queue))
				s.queue = nil
				break
			}

			interval *= 2 // 重试间隔翻倍
			if interval > s.Config.MaxRetryInterval {
				interval = s.Config.MaxRetryInterval
			}
		}

		s.mu.Lock()
		if err == nil {
			interval = s.Config.MinRetryInterval
			deadline = time.Now().Add(s.Config.RetryTimeout)

			s.seq += uint64(n)
			s.queue = s.queue[n:]

			if close {
				break
			}
		}
	}

	s.writing = false
	if s.draining {
		s.Stream.Close(nil)
	}
	if err != nil && s.CloseOnFailure {
		s.Stream.Close(ErrSignalWriteFailed)
	}
	s.mu.Unlock()
}

// 写入消息的公开接口，将消息写入到消息队列中，可能触发异步写入操作
func (s *signalMessageSink[SendType, RecvType]) WriteMessage(msg proto.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Stream.Err(); err != nil {
		return err
	} else if s.draining {
		return psrpc.ErrStreamClosed
	}

	s.queue = append(s.queue, msg)
	if !s.writing {
		s.writing = true
		go s.write()
	}
	return nil
}

func (s *signalMessageSink[SendType, RecvType]) ConnectionID() livekit.ConnectionID {
	return s.SignalSinkParams.ConnectionID
}
