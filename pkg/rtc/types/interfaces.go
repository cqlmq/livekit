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

package types

import (
	"fmt"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/utils"

	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/pkg/sfu"
	"github.com/livekit/livekit-server/pkg/sfu/buffer"
	"github.com/livekit/livekit-server/pkg/sfu/mime"
	"github.com/livekit/livekit-server/pkg/sfu/pacer"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate . WebsocketClient
type WebsocketClient interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetReadDeadline(deadline time.Time) error
	Close() error
}

type AddSubscriberParams struct {
	AllTracks bool
	TrackIDs  []livekit.TrackID
}

// ---------------------------------------------

type MigrateState int32

const (
	MigrateStateInit MigrateState = iota
	MigrateStateSync
	MigrateStateComplete
)

func (m MigrateState) String() string {
	switch m {
	case MigrateStateInit:
		return "MIGRATE_STATE_INIT"
	case MigrateStateSync:
		return "MIGRATE_STATE_SYNC"
	case MigrateStateComplete:
		return "MIGRATE_STATE_COMPLETE"
	default:
		return fmt.Sprintf("%d", int(m))
	}
}

// ---------------------------------------------

type SubscribedCodecQuality struct {
	CodecMime mime.MimeType
	Quality   livekit.VideoQuality
}

// ---------------------------------------------

// ParticipantCloseReason 表示参与者关闭的原因
type ParticipantCloseReason int

const (
	ParticipantCloseReasonNone                            ParticipantCloseReason = iota // 无
	ParticipantCloseReasonClientRequestLeave                                            // 客户端请求离开
	ParticipantCloseReasonRoomManagerStop                                               // 房间管理器停止
	ParticipantCloseReasonVerifyFailed                                                  // 验证失败
	ParticipantCloseReasonJoinFailed                                                    // 加入失败
	ParticipantCloseReasonJoinTimeout                                                   // 加入超时
	ParticipantCloseReasonMessageBusFailed                                              // 消息总线失败
	ParticipantCloseReasonPeerConnectionDisconnected                                    // 对等连接断开
	ParticipantCloseReasonDuplicateIdentity                                             // 重复身份
	ParticipantCloseReasonMigrationComplete                                             // 迁移完成
	ParticipantCloseReasonStale                                                         // 过时
	ParticipantCloseReasonServiceRequestRemoveParticipant                               // 服务请求移除参与者
	ParticipantCloseReasonServiceRequestDeleteRoom                                      // 服务请求删除房间
	ParticipantCloseReasonSimulateMigration                                             // 模拟迁移
	ParticipantCloseReasonSimulateNodeFailure                                           // 模拟节点失败
	ParticipantCloseReasonSimulateServerLeave                                           // 模拟服务器离开
	ParticipantCloseReasonSimulateLeaveRequest                                          // 模拟离开请求
	ParticipantCloseReasonNegotiateFailed                                               // 协商失败
	ParticipantCloseReasonMigrationRequested                                            // 迁移请求
	ParticipantCloseReasonPublicationError                                              // 发布错误
	ParticipantCloseReasonSubscriptionError                                             // 订阅错误
	ParticipantCloseReasonDataChannelError                                              // 数据通道错误
	ParticipantCloseReasonMigrateCodecMismatch                                          // 迁移编码器不匹配
	ParticipantCloseReasonSignalSourceClose                                             // 信号源关闭
	ParticipantCloseReasonRoomClosed                                                    // 房间关闭
	ParticipantCloseReasonUserUnavailable                                               // 用户不可用
	ParticipantCloseReasonUserRejected                                                  // 用户拒绝
)

func (p ParticipantCloseReason) String() string {
	switch p {
	case ParticipantCloseReasonNone:
		return "NONE"
	case ParticipantCloseReasonClientRequestLeave:
		return "CLIENT_REQUEST_LEAVE"
	case ParticipantCloseReasonRoomManagerStop:
		return "ROOM_MANAGER_STOP"
	case ParticipantCloseReasonVerifyFailed:
		return "VERIFY_FAILED"
	case ParticipantCloseReasonJoinFailed:
		return "JOIN_FAILED"
	case ParticipantCloseReasonJoinTimeout:
		return "JOIN_TIMEOUT"
	case ParticipantCloseReasonMessageBusFailed:
		return "MESSAGE_BUS_FAILED"
	case ParticipantCloseReasonPeerConnectionDisconnected:
		return "PEER_CONNECTION_DISCONNECTED"
	case ParticipantCloseReasonDuplicateIdentity:
		return "DUPLICATE_IDENTITY"
	case ParticipantCloseReasonMigrationComplete:
		return "MIGRATION_COMPLETE"
	case ParticipantCloseReasonStale:
		return "STALE"
	case ParticipantCloseReasonServiceRequestRemoveParticipant:
		return "SERVICE_REQUEST_REMOVE_PARTICIPANT"
	case ParticipantCloseReasonServiceRequestDeleteRoom:
		return "SERVICE_REQUEST_DELETE_ROOM"
	case ParticipantCloseReasonSimulateMigration:
		return "SIMULATE_MIGRATION"
	case ParticipantCloseReasonSimulateNodeFailure:
		return "SIMULATE_NODE_FAILURE"
	case ParticipantCloseReasonSimulateServerLeave:
		return "SIMULATE_SERVER_LEAVE"
	case ParticipantCloseReasonSimulateLeaveRequest:
		return "SIMULATE_LEAVE_REQUEST"
	case ParticipantCloseReasonNegotiateFailed:
		return "NEGOTIATE_FAILED"
	case ParticipantCloseReasonMigrationRequested:
		return "MIGRATION_REQUESTED"
	case ParticipantCloseReasonPublicationError:
		return "PUBLICATION_ERROR"
	case ParticipantCloseReasonSubscriptionError:
		return "SUBSCRIPTION_ERROR"
	case ParticipantCloseReasonDataChannelError:
		return "DATA_CHANNEL_ERROR"
	case ParticipantCloseReasonMigrateCodecMismatch:
		return "MIGRATE_CODEC_MISMATCH"
	case ParticipantCloseReasonSignalSourceClose:
		return "SIGNAL_SOURCE_CLOSE"
	case ParticipantCloseReasonRoomClosed:
		return "ROOM_CLOSED"
	case ParticipantCloseReasonUserUnavailable:
		return "USER_UNAVAILABLE"
	case ParticipantCloseReasonUserRejected:
		return "USER_REJECTED"
	default:
		return fmt.Sprintf("%d", int(p))
	}
}

func (p ParticipantCloseReason) ToDisconnectReason() livekit.DisconnectReason {
	switch p {
	case ParticipantCloseReasonClientRequestLeave, ParticipantCloseReasonSimulateLeaveRequest:
		return livekit.DisconnectReason_CLIENT_INITIATED
	case ParticipantCloseReasonRoomManagerStop:
		return livekit.DisconnectReason_SERVER_SHUTDOWN
	case ParticipantCloseReasonVerifyFailed, ParticipantCloseReasonJoinFailed, ParticipantCloseReasonJoinTimeout, ParticipantCloseReasonMessageBusFailed:
		// expected to be connected but is not
		return livekit.DisconnectReason_JOIN_FAILURE
	case ParticipantCloseReasonPeerConnectionDisconnected:
		return livekit.DisconnectReason_STATE_MISMATCH
	case ParticipantCloseReasonDuplicateIdentity, ParticipantCloseReasonStale:
		return livekit.DisconnectReason_DUPLICATE_IDENTITY
	case ParticipantCloseReasonMigrationRequested, ParticipantCloseReasonMigrationComplete, ParticipantCloseReasonSimulateMigration:
		return livekit.DisconnectReason_MIGRATION
	case ParticipantCloseReasonServiceRequestRemoveParticipant:
		return livekit.DisconnectReason_PARTICIPANT_REMOVED
	case ParticipantCloseReasonServiceRequestDeleteRoom:
		return livekit.DisconnectReason_ROOM_DELETED
	case ParticipantCloseReasonSimulateNodeFailure, ParticipantCloseReasonSimulateServerLeave:
		return livekit.DisconnectReason_SERVER_SHUTDOWN
	case ParticipantCloseReasonNegotiateFailed, ParticipantCloseReasonPublicationError, ParticipantCloseReasonSubscriptionError, ParticipantCloseReasonDataChannelError, ParticipantCloseReasonMigrateCodecMismatch:
		return livekit.DisconnectReason_STATE_MISMATCH
	case ParticipantCloseReasonSignalSourceClose:
		return livekit.DisconnectReason_SIGNAL_CLOSE
	case ParticipantCloseReasonRoomClosed:
		return livekit.DisconnectReason_ROOM_CLOSED
	case ParticipantCloseReasonUserUnavailable:
		return livekit.DisconnectReason_USER_UNAVAILABLE
	case ParticipantCloseReasonUserRejected:
		return livekit.DisconnectReason_USER_REJECTED
	default:
		// the other types will map to unknown reason
		return livekit.DisconnectReason_UNKNOWN_REASON
	}
}

// ---------------------------------------------

// SignallingCloseReason 表示信令关闭的原因
type SignallingCloseReason int

const (
	SignallingCloseReasonUnknown                        SignallingCloseReason = iota // 未知
	SignallingCloseReasonMigration                                                   // 迁移
	SignallingCloseReasonResume                                                      // 恢复
	SignallingCloseReasonTransportFailure                                            // 传输失败
	SignallingCloseReasonFullReconnectPublicationError                               // 全连接发布错误
	SignallingCloseReasonFullReconnectSubscriptionError                              // 全连接订阅错误
	SignallingCloseReasonFullReconnectDataChannelError                               // 全连接数据通道错误
	SignallingCloseReasonFullReconnectNegotiateFailed                                // 全连接协商失败
	SignallingCloseReasonParticipantClose                                            // 参与者关闭
	SignallingCloseReasonDisconnectOnResume                                          // 断开连接恢复
	SignallingCloseReasonDisconnectOnResumeNoMessages                                // 断开连接恢复没有消息
)

func (s SignallingCloseReason) String() string {
	switch s {
	case SignallingCloseReasonUnknown:
		return "UNKNOWN"
	case SignallingCloseReasonMigration:
		return "MIGRATION"
	case SignallingCloseReasonResume:
		return "RESUME"
	case SignallingCloseReasonTransportFailure:
		return "TRANSPORT_FAILURE"
	case SignallingCloseReasonFullReconnectPublicationError:
		return "FULL_RECONNECT_PUBLICATION_ERROR"
	case SignallingCloseReasonFullReconnectSubscriptionError:
		return "FULL_RECONNECT_SUBSCRIPTION_ERROR"
	case SignallingCloseReasonFullReconnectDataChannelError:
		return "FULL_RECONNECT_DATA_CHANNEL_ERROR"
	case SignallingCloseReasonFullReconnectNegotiateFailed:
		return "FULL_RECONNECT_NEGOTIATE_FAILED"
	case SignallingCloseReasonParticipantClose:
		return "PARTICIPANT_CLOSE"
	case SignallingCloseReasonDisconnectOnResume:
		return "DISCONNECT_ON_RESUME"
	case SignallingCloseReasonDisconnectOnResumeNoMessages:
		return "DISCONNECT_ON_RESUME_NO_MESSAGES"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}

// ---------------------------------------------

//counterfeiter:generate . Participant 参与者接口
type Participant interface {
	ID() livekit.ParticipantID             // 参与者ID
	Identity() livekit.ParticipantIdentity // 参与者身份
	State() livekit.ParticipantInfo_State  // 参与者状态
	ConnectedAt() time.Time                // 连接时间
	CloseReason() ParticipantCloseReason   // 关闭原因
	Kind() livekit.ParticipantInfo_Kind    // 参与者类型
	IsRecorder() bool                      // 是否为记录器
	IsDependent() bool                     // 是否为依赖
	IsAgent() bool                         // 是否为代理

	CanSkipBroadcast() bool            // 是否可以跳过广播
	Version() utils.TimedVersion       // 版本
	ToProto() *livekit.ParticipantInfo // 转换为参与者信息

	IsPublisher() bool                                                                // 是否为发布者
	GetPublishedTrack(trackID livekit.TrackID) MediaTrack                             // 获取已发布的轨道
	GetPublishedTracks() []MediaTrack                                                 // 获取已发布的轨道列表
	RemovePublishedTrack(track MediaTrack, isExpectedToResume bool, shouldClose bool) // 移除已发布的轨道

	GetAudioLevel() (smoothedLevel float64, active bool) // 获取音频级别

	// HasPermission checks permission of the subscriber by identity. Returns true if subscriber is allowed to subscribe
	// to the track with trackID
	HasPermission(trackID livekit.TrackID, subIdentity livekit.ParticipantIdentity) bool // 检查订阅者的权限

	// permissions
	Hidden() bool // 是否隐藏

	Close(sendLeave bool, reason ParticipantCloseReason, isExpectedToResume bool) error // 关闭

	SubscriptionPermission() (*livekit.SubscriptionPermission, utils.TimedVersion) // 订阅权限

	// updates from remotes
	UpdateSubscriptionPermission(
		subscriptionPermission *livekit.SubscriptionPermission, // 订阅权限
		timedVersion utils.TimedVersion, // 时间版本
		resolverBySid func(participantID livekit.ParticipantID) LocalParticipant, // 解析器
	) error // 更新订阅权限

	DebugInfo() map[string]interface{} // 调试信息

	OnMetrics(callback func(Participant, *livekit.DataPacket)) // 指标回调
}

// -------------------------------------------------------
// 添加轨道参数
type AddTrackParams struct {
	Stereo bool // 立体声
	Red    bool // 红色, 意思是？？
}

//counterfeiter:generate . LocalParticipant 本地参与者接口
type LocalParticipant interface {
	Participant // 继承参与者接口

	ToProtoWithVersion() (*livekit.ParticipantInfo, utils.TimedVersion) // 转换为参与者信息

	// getters 获取器
	GetTrailer() []byte                                         // 获取尾部
	GetLogger() logger.Logger                                   // 获取日志
	GetAdaptiveStream() bool                                    // 获取自适应流
	ProtocolVersion() ProtocolVersion                           // 协议版本
	SupportsSyncStreamID() bool                                 // 支持同步流ID
	SupportsTransceiverReuse() bool                             // 支持转换器重用
	IsClosed() bool                                             // 是否关闭
	IsReady() bool                                              // 是否准备就绪
	IsDisconnected() bool                                       // 是否断开连接
	Disconnected() <-chan struct{}                              // 断开连接通道
	IsIdle() bool                                               // 是否空闲
	SubscriberAsPrimary() bool                                  // 订阅者是否为主
	GetClientInfo() *livekit.ClientInfo                         // 获取客户端信息
	GetClientConfiguration() *livekit.ClientConfiguration       // 获取客户端配置
	GetBufferFactory() *buffer.Factory                          // 获取缓冲区工厂
	GetPlayoutDelayConfig() *livekit.PlayoutDelay               // 获取播放延迟配置
	GetPendingTrack(trackID livekit.TrackID) *livekit.TrackInfo // 获取待处理的轨道
	GetICEConnectionInfo() []*ICEConnectionInfo                 // 获取ICE连接信息
	HasConnected() bool                                         // 是否连接
	GetEnabledPublishCodecs() []*livekit.Codec                  // 获取启用的发布编码器

	SetResponseSink(sink routing.MessageSink)           // 设置响应接收器
	CloseSignalConnection(reason SignallingCloseReason) // 关闭信令连接
	UpdateLastSeenSignal()                              // 更新最后看到的信令
	SetSignalSourceValid(valid bool)                    // 设置信令源有效
	HandleSignalSourceClose()                           // 处理信令源关闭

	// updates 更新
	CheckMetadataLimits(name string, metadata string, attributes map[string]string) error // 检查元数据限制
	SetName(name string)                                                                  // 设置名称
	SetMetadata(metadata string)                                                          // 设置元数据
	SetAttributes(attributes map[string]string)                                           // 设置属性
	UpdateAudioTrack(update *livekit.UpdateLocalAudioTrack) error                         // 更新音频轨道
	UpdateVideoTrack(update *livekit.UpdateLocalVideoTrack) error                         // 更新视频轨道

	// permissions 权限
	ClaimGrants() *auth.ClaimGrants                               // 声明授权
	SetPermission(permission *livekit.ParticipantPermission) bool // 设置权限
	CanPublish() bool                                             // 是否可以发布
	CanPublishSource(source livekit.TrackSource) bool             // 是否可以发布源
	CanSubscribe() bool                                           // 是否可以订阅
	CanPublishData() bool                                         // 是否可以发布数据

	// PeerConnection 对等连接
	AddICECandidate(candidate webrtc.ICECandidateInit, target livekit.SignalTarget)       // 添加ICE候选者
	HandleOffer(sdp webrtc.SessionDescription) error                                      // 处理Offer
	GetAnswer() (webrtc.SessionDescription, error)                                        // 获取Answer
	AddTrack(req *livekit.AddTrackRequest)                                                // 添加轨道
	SetTrackMuted(trackID livekit.TrackID, muted bool, fromAdmin bool) *livekit.TrackInfo // 设置轨道静音

	HandleAnswer(sdp webrtc.SessionDescription)                                                                                          // 处理Answer
	Negotiate(force bool)                                                                                                                // 协商
	ICERestart(iceConfig *livekit.ICEConfig)                                                                                             // 重启ICE
	AddTrackLocal(trackLocal webrtc.TrackLocal, params AddTrackParams) (*webrtc.RTPSender, *webrtc.RTPTransceiver, error)                // 添加本地轨道
	AddTransceiverFromTrackLocal(trackLocal webrtc.TrackLocal, params AddTrackParams) (*webrtc.RTPSender, *webrtc.RTPTransceiver, error) // 添加本地轨道
	RemoveTrackLocal(sender *webrtc.RTPSender) error                                                                                     // 移除本地轨道

	WriteSubscriberRTCP(pkts []rtcp.Packet) error // 写入订阅者RTCP

	// subscriptions 订阅
	SubscribeToTrack(trackID livekit.TrackID)                                                     // 订阅轨道
	UnsubscribeFromTrack(trackID livekit.TrackID)                                                 // 取消订阅轨道
	UpdateSubscribedTrackSettings(trackID livekit.TrackID, settings *livekit.UpdateTrackSettings) // 更新订阅轨道设置
	GetSubscribedTracks() []SubscribedTrack                                                       // 获取订阅轨道
	IsTrackNameSubscribed(publisherIdentity livekit.ParticipantIdentity, trackName string) bool   // 是否订阅轨道名称
	Verify() bool                                                                                 // 验证
	VerifySubscribeParticipantInfo(pID livekit.ParticipantID, version uint32)                     // 验证订阅参与者信息
	// WaitUntilSubscribed waits until all subscriptions have been settled, or if the timeout
	// has been reached. If the timeout expires, it will return an error.
	WaitUntilSubscribed(timeout time.Duration) error                                          // 等待订阅
	StopAndGetSubscribedTracksForwarderState() map[livekit.TrackID]*livekit.RTPForwarderState // 停止并获取订阅轨道转发器状态
	SupportsCodecChange() bool                                                                // 支持编码器改变

	// returns list of participant identities that the current participant is subscribed to
	GetSubscribedParticipants() []livekit.ParticipantID // 获取订阅参与者
	IsSubscribedTo(sid livekit.ParticipantID) bool      // 是否订阅

	GetConnectionQuality() *livekit.ConnectionQualityInfo // 获取连接质量

	// server sent messages 服务器发送消息
	SendJoinResponse(joinResponse *livekit.JoinResponse) error                                                                  // 发送加入响应
	SendParticipantUpdate(participants []*livekit.ParticipantInfo) error                                                        // 发送参与者更新
	SendSpeakerUpdate(speakers []*livekit.SpeakerInfo, force bool) error                                                        // 发送发言人更新
	SendDataPacket(kind livekit.DataPacket_Kind, encoded []byte) error                                                          // 发送数据包
	SendRoomUpdate(room *livekit.Room) error                                                                                    // 发送房间更新
	SendConnectionQualityUpdate(update *livekit.ConnectionQualityUpdate) error                                                  // 发送连接质量更新
	SubscriptionPermissionUpdate(publisherID livekit.ParticipantID, trackID livekit.TrackID, allowed bool)                      // 订阅权限更新
	SendRefreshToken(token string) error                                                                                        // 发送刷新令牌
	SendRequestResponse(requestResponse *livekit.RequestResponse) error                                                         // 发送请求响应
	HandleReconnectAndSendResponse(reconnectReason livekit.ReconnectReason, reconnectResponse *livekit.ReconnectResponse) error // 处理重连并发送响应
	IssueFullReconnect(reason ParticipantCloseReason)                                                                           // 发送全连接

	// callbacks 回调
	OnStateChange(func(p LocalParticipant, state livekit.ParticipantInfo_State)) // 状态变化
	OnMigrateStateChange(func(p LocalParticipant, migrateState MigrateState))    // 迁移状态变化
	// OnTrackPublished - remote added a track 远程添加轨道
	OnTrackPublished(func(LocalParticipant, MediaTrack)) // 轨道发布
	// OnTrackUpdated - one of its publishedTracks changed in status
	OnTrackUpdated(callback func(LocalParticipant, MediaTrack)) // 轨道更新
	// OnTrackUnpublished - a track was unpublished
	OnTrackUnpublished(callback func(LocalParticipant, MediaTrack)) // 轨道取消发布
	// OnParticipantUpdate - metadata or permission is updated
	OnParticipantUpdate(callback func(LocalParticipant))                                        // 参与者更新
	OnDataPacket(callback func(LocalParticipant, livekit.DataPacket_Kind, *livekit.DataPacket)) // 数据包
	OnSubscribeStatusChanged(fn func(publisherID livekit.ParticipantID, subscribed bool))       // 订阅状态变化
	OnClose(callback func(LocalParticipant))                                                    // 关闭
	OnClaimsChanged(callback func(LocalParticipant))                                            // 声明变化

	HandleReceiverReport(dt *sfu.DownTrack, report *rtcp.ReceiverReport)

	// session migration 会话迁移
	MaybeStartMigration(force bool, onStart func()) bool // 可能开始迁移
	NotifyMigration()                                    // 通知迁移
	SetMigrateState(s MigrateState)                      // 设置迁移状态
	MigrateState() MigrateState                          // 迁移状态
	SetMigrateInfo(
		previousOffer, previousAnswer *webrtc.SessionDescription, // 之前的Offer和Answer
		mediaTracks []*livekit.TrackPublishedResponse, // 媒体轨道
		dataChannels []*livekit.DataChannelInfo, // 数据通道
	)
	IsReconnect() bool // 是否重连

	UpdateMediaRTT(rtt uint32)     // 更新媒体RTT
	UpdateSignalingRTT(rtt uint32) // 更新信令RTT

	CacheDownTrack(trackID livekit.TrackID, rtpTransceiver *webrtc.RTPTransceiver, downTrackState sfu.DownTrackState) // 缓存下行轨道
	UncacheDownTrack(rtpTransceiver *webrtc.RTPTransceiver)                                                           // 取消缓存下行轨道
	GetCachedDownTrack(trackID livekit.TrackID) (*webrtc.RTPTransceiver, sfu.DownTrackState)                          // 获取缓存下行轨道

	SetICEConfig(iceConfig *livekit.ICEConfig)                                                    // 设置ICE配置
	GetICEConfig() *livekit.ICEConfig                                                             // 获取ICE配置
	OnICEConfigChanged(callback func(participant LocalParticipant, iceConfig *livekit.ICEConfig)) // ICE配置变化

	UpdateSubscribedQuality(nodeID livekit.NodeID, trackID livekit.TrackID, maxQualities []SubscribedCodecQuality) error // 更新订阅质量
	UpdateMediaLoss(nodeID livekit.NodeID, trackID livekit.TrackID, fractionalLoss uint32) error

	// down stream bandwidth management 下行带宽管理
	SetSubscriberAllowPause(allowPause bool)            // 设置订阅者允许暂停
	SetSubscriberChannelCapacity(channelCapacity int64) // 设置订阅者通道容量

	GetPacer() pacer.Pacer // 获取分隔器

	GetDisableSenderReportPassThrough() bool // 获取禁用发送者报告传递

	HandleMetrics(senderParticipantID livekit.ParticipantID, batch *livekit.MetricsBatch) error // 处理指标
}

// Room is a container of participants, and can provide room-level actions
// 房间是参与者的容器，可以提供房间级别的操作
//
//counterfeiter:generate . Room
type Room interface {
	Name() livekit.RoomName                                                                                                                       // 房间名称
	ID() livekit.RoomID                                                                                                                           // 房间ID
	RemoveParticipant(identity livekit.ParticipantIdentity, pID livekit.ParticipantID, reason ParticipantCloseReason)                             // 移除参与者
	UpdateSubscriptions(participant LocalParticipant, trackIDs []livekit.TrackID, participantTracks []*livekit.ParticipantTracks, subscribe bool) // 更新订阅
	UpdateSubscriptionPermission(participant LocalParticipant, permissions *livekit.SubscriptionPermission) error                                 // 更新订阅权限
	SyncState(participant LocalParticipant, state *livekit.SyncState) error                                                                       // 同步状态
	SimulateScenario(participant LocalParticipant, scenario *livekit.SimulateScenario) error                                                      // 模拟场景
	ResolveMediaTrackForSubscriber(sub LocalParticipant, trackID livekit.TrackID) MediaResolverResult                                             // 解析媒体轨道
	GetLocalParticipants() []LocalParticipant                                                                                                     // 获取本地参与者
	IsDataMessageUserPacketDuplicate(ip *livekit.UserPacket) bool                                                                                 // 是否是数据消息用户包重复
}

// MediaTrack represents a media track
// 媒体轨道表示一个媒体轨道
//
//counterfeiter:generate . MediaTrack
type MediaTrack interface {
	ID() livekit.TrackID         // 轨道ID
	Kind() livekit.TrackType     // 轨道类型
	Name() string                // 名称
	Source() livekit.TrackSource // 源
	Stream() string              // 流

	UpdateTrackInfo(ti *livekit.TrackInfo)                  // 更新轨道信息
	UpdateAudioTrack(update *livekit.UpdateLocalAudioTrack) // 更新音频轨道
	UpdateVideoTrack(update *livekit.UpdateLocalVideoTrack) // 更新视频轨道
	ToProto() *livekit.TrackInfo                            // 转换为轨道信息

	PublisherID() livekit.ParticipantID             // 发布者ID
	PublisherIdentity() livekit.ParticipantIdentity // 发布者身份
	PublisherVersion() uint32                       // 发布者版本

	IsMuted() bool       // 是否静音
	SetMuted(muted bool) // 设置静音

	IsSimulcast() bool // 是否是多流

	GetAudioLevel() (level float64, active bool) // 获取音频级别

	Close(isExpectedToResume bool) // 关闭
	IsOpen() bool                  // 是否打开

	// callbacks
	AddOnClose(func(isExpectedToResume bool)) // 添加关闭回调

	// subscribers
	AddSubscriber(participant LocalParticipant) (SubscribedTrack, error)                                                 // 添加订阅者
	RemoveSubscriber(participantID livekit.ParticipantID, isExpectedToResume bool)                                       // 移除订阅者
	IsSubscriber(subID livekit.ParticipantID) bool                                                                       // 是否是订阅者
	RevokeDisallowedSubscribers(allowedSubscriberIdentities []livekit.ParticipantIdentity) []livekit.ParticipantIdentity // 撤销不允许的订阅者
	GetAllSubscribers() []livekit.ParticipantID                                                                          // 获取所有订阅者
	GetNumSubscribers() int                                                                                              // 获取订阅者数量
	OnTrackSubscribed()                                                                                                  // 轨道订阅

	// returns quality information that's appropriate for width & height
	GetQualityForDimension(width, height uint32) livekit.VideoQuality // 获取适合宽度和高度的质量

	// returns temporal layer that's appropriate for fps
	GetTemporalLayerForSpatialFps(spatial int32, fps uint32, mime mime.MimeType) int32

	Receivers() []sfu.TrackReceiver            // 接收者
	ClearAllReceivers(isExpectedToResume bool) // 清除所有接收者

	IsEncrypted() bool // 是否加密
}

//counterfeiter:generate . LocalMediaTrack 本地媒体轨道接口
type LocalMediaTrack interface {
	MediaTrack // 继承媒体轨道接口

	Restart() // 重启

	SignalCid() string         // 信号CID
	HasSdpCid(cid string) bool // 是否包含SDP CID

	GetConnectionScoreAndQuality() (float32, livekit.ConnectionQuality) // 获取连接得分和质量
	GetTrackStats() *livekit.RTPStats                                   // 获取轨道统计

	SetRTT(rtt uint32) // 设置RTT

	NotifySubscriberNodeMaxQuality(nodeID livekit.NodeID, qualities []SubscribedCodecQuality) // 通知订阅者节点最大质量
	NotifySubscriberNodeMediaLoss(nodeID livekit.NodeID, fractionalLoss uint8)                // 通知订阅者节点媒体损失
}

//counterfeiter:generate . SubscribedTrack 订阅轨道接口
type SubscribedTrack interface {
	AddOnBind(f func(error))                                                          // 添加绑定回调
	IsBound() bool                                                                    // 是否绑定
	Close(isExpectedToResume bool)                                                    // 关闭
	OnClose(f func(isExpectedToResume bool))                                          // 关闭回调
	ID() livekit.TrackID                                                              // 轨道ID
	PublisherID() livekit.ParticipantID                                               // 发布者ID
	PublisherIdentity() livekit.ParticipantIdentity                                   // 发布者身份
	PublisherVersion() uint32                                                         // 发布者版本
	SubscriberID() livekit.ParticipantID                                              // 订阅者ID
	SubscriberIdentity() livekit.ParticipantIdentity                                  // 订阅者身份
	Subscriber() LocalParticipant                                                     // 订阅者
	DownTrack() *sfu.DownTrack                                                        // 下行轨道
	MediaTrack() MediaTrack                                                           // 媒体轨道
	RTPSender() *webrtc.RTPSender                                                     // RTPSender
	IsMuted() bool                                                                    // 是否静音
	SetPublisherMuted(muted bool)                                                     // 设置发布者静音
	UpdateSubscriberSettings(settings *livekit.UpdateTrackSettings, isImmediate bool) // 更新订阅者设置
	// selects appropriate video layer according to subscriber preferences
	UpdateVideoLayer()      // 更新视频层
	NeedsNegotiation() bool // 需要协商
}

// ChangeNotifier 变化通知器接口
type ChangeNotifier interface {
	AddObserver(key string, onChanged func()) // 添加观察者
	RemoveObserver(key string)                // 移除观察者
	HasObservers() bool                       // 是否有观察者
	NotifyChanged()                           // 通知变化
}

// MediaResolverResult 媒体解析器结果
type MediaResolverResult struct {
	TrackChangedNotifier ChangeNotifier // 轨道变化通知器
	TrackRemovedNotifier ChangeNotifier // 轨道移除通知器
	Track                MediaTrack     // 媒体轨道
	// is permission given to the requesting participant
	HasPermission     bool                        // 是否授予权限
	PublisherID       livekit.ParticipantID       // 发布者ID
	PublisherIdentity livekit.ParticipantIdentity // 发布者身份
}

// MediaTrackResolver locates a specific media track for a subscriber // 媒体轨道解析器 定位一个特定的媒体轨道给订阅者
type MediaTrackResolver func(LocalParticipant, livekit.TrackID) MediaResolverResult

// Supervisor/operation monitor related definitions // 主管/操作监控相关定义
type OperationMonitorEvent int

const (
	OperationMonitorEventPublisherPeerConnectionConnected OperationMonitorEvent = iota // 发布者对等连接已连接
	OperationMonitorEventAddPendingPublication                                         // 添加待发布
	OperationMonitorEventSetPublicationMute                                            // 设置发布静音
	OperationMonitorEventSetPublishedTrack                                             // 设置发布轨道
	OperationMonitorEventClearPublishedTrack                                           // 清除发布轨道
)

func (o OperationMonitorEvent) String() string {
	switch o {
	case OperationMonitorEventPublisherPeerConnectionConnected:
		return "PUBLISHER_PEER_CONNECTION_CONNECTED"
	case OperationMonitorEventAddPendingPublication:
		return "ADD_PENDING_PUBLICATION"
	case OperationMonitorEventSetPublicationMute:
		return "SET_PUBLICATION_MUTE"
	case OperationMonitorEventSetPublishedTrack:
		return "SET_PUBLISHED_TRACK"
	case OperationMonitorEventClearPublishedTrack:
		return "CLEAR_PUBLISHED_TRACK"
	default:
		return fmt.Sprintf("%d", int(o))
	}
}

// OperationMonitorData 操作监控数据
type OperationMonitorData interface{}

// OperationMonitor 操作监控接口
type OperationMonitor interface {
	PostEvent(ome OperationMonitorEvent, omd OperationMonitorData) // 发布事件
	Check() error                                                  // 检查
	IsIdle() bool                                                  // 是否空闲
}
