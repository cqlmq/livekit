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

package rtc

import (
	"errors"
)

var (
	ErrRoomClosed               = errors.New("room has already closed")                                     // 房间已关闭
	ErrParticipantSessionClosed = errors.New("participant session is already closed")                       // 参与者会话已关闭
	ErrPermissionDenied         = errors.New("no permissions to access the room")                           // 没有权限访问房间
	ErrMaxParticipantsExceeded  = errors.New("room has exceeded its max participants")                      // 房间已超过最大参与者数量
	ErrLimitExceeded            = errors.New("node has exceeded its configured limit")                      // 节点已超过其配置的限制
	ErrAlreadyJoined            = errors.New("a participant with the same identity is already in the room") // 已存在相同身份的参与者
	ErrDataChannelUnavailable   = errors.New("data channel is not available")                               // 数据通道不可用
	ErrDataChannelBufferFull    = errors.New("data channel buffer is full")                                 // 数据通道缓冲区已满
	ErrTransportFailure         = errors.New("transport failure")                                           // 传输失败
	ErrEmptyIdentity            = errors.New("participant identity cannot be empty")                        // 参与者身份不能为空
	ErrEmptyParticipantID       = errors.New("participant ID cannot be empty")                              // 参与者ID不能为空
	ErrMissingGrants            = errors.New("VideoGrant is missing")                                       // VideoGrant缺失
	ErrInternalError            = errors.New("internal error")                                              // 内部错误
	ErrNameExceedsLimits        = errors.New("name length exceeds limits")                                  // 名称长度超过限制
	ErrMetadataExceedsLimits    = errors.New("metadata size exceeds limits")                                // 元数据大小超过限制
	ErrAttributesExceedsLimits  = errors.New("attributes size exceeds limits")                              // 属性大小超过限制

	// Track subscription related
	ErrNoTrackPermission         = errors.New("participant is not allowed to subscribe to this track")      // 参与者不允许订阅此轨道
	ErrNoSubscribePermission     = errors.New("participant is not given permission to subscribe to tracks") // 参与者没有权限订阅轨道
	ErrTrackNotFound             = errors.New("track cannot be found")                                      // 轨道无法找到
	ErrTrackNotBound             = errors.New("track not bound")                                            // 轨道未绑定
	ErrSubscriptionLimitExceeded = errors.New("participant has exceeded its subscription limit")            // 参与者已超过其订阅限制

	ErrNoSubscribeMetricsPermission = errors.New("participant is not given permission to subscribe to metrics") // 参与者没有权限订阅指标
)
