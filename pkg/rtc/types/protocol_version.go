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

// 定义了当前使用的协议版本号（CurrentProtocol = 15）
// 通过一系列的方法来判断不同协议版本支持的功能特性，比如：
// 是否支持打包的流ID（PackedStreamId）
// 是否支持 Protobuf
// 是否支持数据包处理
// 是否支持订阅者作为主要连接
// 是否支持扬声器变更通知
// 是否支持收发器重用
// 是否支持连接质量监控
// 是否支持会话迁移
// 是否支持 ICE Lite
// 是否支持取消发布
// 是否支持快速启动

// 这种设计模式非常适合分布式系统中的版本管理，因为它：
// 确保了不同版本的客户端和服务器之间的兼容性
// 允许系统逐步演进，同时保持对旧版本的支持
// 提供了清晰的功能支持判断机制

type ProtocolVersion int

// 当前协议版本号
const CurrentProtocol = 15

// 支持打包的流ID
func (v ProtocolVersion) SupportsPackedStreamId() bool {
	return v > 0
}

// 支持 Protobuf
func (v ProtocolVersion) SupportsProtobuf() bool {
	return v > 0
}

// 支持数据包处理
func (v ProtocolVersion) HandlesDataPackets() bool {
	return v > 1
}

// 订阅者作为主要连接
func (v ProtocolVersion) SubscriberAsPrimary() bool {
	return v > 2
}

// 支持扬声器变更通知
func (v ProtocolVersion) SupportsSpeakerChanged() bool {
	return v > 2
}

// 支持收发器重用
func (v ProtocolVersion) SupportsTransceiverReuse() bool {
	return v > 3
}

// 支持连接质量监控
func (v ProtocolVersion) SupportsConnectionQuality() bool {
	return v > 4
}

// 支持会话迁移
func (v ProtocolVersion) SupportsSessionMigrate() bool {
	return v > 5
}

// 支持 ICE Lite
func (v ProtocolVersion) SupportsICELite() bool {
	return v > 5
}

// 支持取消发布
func (v ProtocolVersion) SupportsUnpublish() bool {
	return v > 6
}

// 支持快速启动
func (v ProtocolVersion) SupportFastStart() bool {
	return v > 7
}

// 支持处理断开连接更新
func (v ProtocolVersion) SupportHandlesDisconnectedUpdate() bool {
	return v > 8
}

// 支持同步流ID
func (v ProtocolVersion) SupportSyncStreamID() bool {
	return v > 9
}

// 支持连接质量丢失
func (v ProtocolVersion) SupportsConnectionQualityLost() bool {
	return v > 10
}

// 支持异步房间ID
func (v ProtocolVersion) SupportsAsyncRoomID() bool {
	return v > 11
}

// 支持基于身份的重新连接
func (v ProtocolVersion) SupportsIdentityBasedReconnection() bool {
	return v > 11
}

// 支持在离开请求中包含区域
func (v ProtocolVersion) SupportsRegionsInLeaveRequest() bool {
	return v > 12
}

// 支持非错误信号响应
func (v ProtocolVersion) SupportsNonErrorSignalResponse() bool {
	return v > 14
}
