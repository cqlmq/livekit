// Copyright 2024 LiveKit, Inc.
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

package transport

import (
	"errors"

	"github.com/pion/webrtc/v4"

	"github.com/livekit/livekit-server/pkg/rtc/types"
	"github.com/livekit/livekit-server/pkg/sfu/streamallocator"
	"github.com/livekit/protocol/livekit"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

var (
	ErrNoICECandidateHandler = errors.New("no ICE candidate handler")
	ErrNoOfferHandler        = errors.New("no offer handler")
	ErrNoAnswerHandler       = errors.New("no answer handler")
)

//counterfeiter:generate . Handler // 生成 Handler 的 counterfeiter 实现
type Handler interface {
	OnICECandidate(c *webrtc.ICECandidate, target livekit.SignalTarget) error // 处理 ICE 候选者
	OnInitialConnected()                                                      // 处理初始连接
	OnFullyEstablished()                                                      // 处理完全建立
	OnFailed(isShortLived bool, iceConnectionInfo *types.ICEConnectionInfo)   // 处理失败
	OnTrack(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver)       // 处理轨道
	OnDataPacket(kind livekit.DataPacket_Kind, data []byte)                   // 处理数据包
	OnDataSendError(err error)                                                // 处理数据发送错误
	OnOffer(sd webrtc.SessionDescription) error                               // 处理 offer
	OnAnswer(sd webrtc.SessionDescription) error                              // 处理 answer
	OnNegotiationStateChanged(state NegotiationState)                         // 处理协商状态变化
	OnNegotiationFailed()                                                     // 处理协商失败
	OnStreamStateChange(update *streamallocator.StreamStateUpdate) error      // 处理流状态变化
}

// UnimplementedHandler 未实现 Handler
type UnimplementedHandler struct{}

// OnICECandidate 处理 ICE 候选者
func (h UnimplementedHandler) OnICECandidate(c *webrtc.ICECandidate, target livekit.SignalTarget) error {
	return ErrNoICECandidateHandler
}
func (h UnimplementedHandler) OnInitialConnected()                                                {}
func (h UnimplementedHandler) OnFullyEstablished()                                                {}
func (h UnimplementedHandler) OnFailed(isShortLived bool)                                         {}
func (h UnimplementedHandler) OnTrack(track *webrtc.TrackRemote, rtpReceiver *webrtc.RTPReceiver) {}
func (h UnimplementedHandler) OnDataPacket(kind livekit.DataPacket_Kind, data []byte)             {}
func (h UnimplementedHandler) OnDataSendError(err error)                                          {}
func (h UnimplementedHandler) OnOffer(sd webrtc.SessionDescription) error {
	return ErrNoOfferHandler
}
func (h UnimplementedHandler) OnAnswer(sd webrtc.SessionDescription) error {
	return ErrNoAnswerHandler
}
func (h UnimplementedHandler) OnNegotiationStateChanged(state NegotiationState) {}
func (h UnimplementedHandler) OnNegotiationFailed()                             {}
func (h UnimplementedHandler) OnStreamStateChange(update *streamallocator.StreamStateUpdate) error {
	return nil
}
