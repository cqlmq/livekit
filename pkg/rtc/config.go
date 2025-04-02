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
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/sfu/buffer"
	dd "github.com/livekit/livekit-server/pkg/sfu/rtpextension/dependencydescriptor"
	"github.com/livekit/mediatransportutil/pkg/rtcconfig"
)

const (
	frameMarking        = "urn:ietf:params:rtp-hdrext:framemarking"
	repairedRTPStreamID = "urn:ietf:params:rtp-hdrext:sdes:repaired-rtp-stream-id"
)

// WebRTCConfig WebRTC 配置
type WebRTCConfig struct {
	rtcconfig.WebRTCConfig // 继承 WebRTC 配置

	BufferFactory *buffer.Factory // 缓冲区工厂
	Receiver      ReceiverConfig  // 接收器配置
	Publisher     DirectionConfig // 发布者配置
	Subscriber    DirectionConfig // 订阅者配置
}

// ReceiverConfig 接收器配置
type ReceiverConfig struct {
	PacketBufferSizeVideo int // 视频缓冲区大小
	PacketBufferSizeAudio int // 音频缓冲区大小
}

// RTPHeaderExtensionConfig RTP 头扩展配置
type RTPHeaderExtensionConfig struct {
	Audio []string // 音频头扩展
	Video []string // 视频头扩展
}

// RTCPFeedbackConfig RTCP 反馈配置
type RTCPFeedbackConfig struct {
	Audio []webrtc.RTCPFeedback // 音频反馈
	Video []webrtc.RTCPFeedback // 视频反馈
}

// DirectionConfig 方向配置
type DirectionConfig struct {
	RTPHeaderExtension RTPHeaderExtensionConfig // RTP 头扩展配置
	RTCPFeedback       RTCPFeedbackConfig       // RTCP 反馈配置
}

// NewWebRTCConfig 创建 WebRTC 配置
func NewWebRTCConfig(conf *config.Config) (*WebRTCConfig, error) {
	rtcConf := conf.RTC // 获取 RTC 配置

	webRTCConfig, err := rtcconfig.NewWebRTCConfig(&rtcConf.RTCConfig, conf.Development)
	if err != nil {
		return nil, err
	}

	// we don't want to use active TCP on a server, clients should be dialing
	webRTCConfig.SettingEngine.DisableActiveTCP(true)

	if rtcConf.PacketBufferSize == 0 {
		rtcConf.PacketBufferSize = 500
	}
	if rtcConf.PacketBufferSizeVideo == 0 {
		rtcConf.PacketBufferSizeVideo = rtcConf.PacketBufferSize
	}
	if rtcConf.PacketBufferSizeAudio == 0 {
		rtcConf.PacketBufferSizeAudio = rtcConf.PacketBufferSize
	}

	// publisher configuration
	publisherConfig := DirectionConfig{
		RTPHeaderExtension: RTPHeaderExtensionConfig{
			Audio: []string{
				sdp.SDESMidURI,
				sdp.SDESRTPStreamIDURI,
				sdp.AudioLevelURI,
				//act.AbsCaptureTimeURI,
			},
			Video: []string{
				sdp.SDESMidURI,
				sdp.SDESRTPStreamIDURI,
				sdp.TransportCCURI,
				frameMarking,
				dd.ExtensionURI,
				repairedRTPStreamID,
				//act.AbsCaptureTimeURI,
			},
		},
		RTCPFeedback: RTCPFeedbackConfig{
			Audio: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBNACK},
			},
			Video: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBTransportCC},
				{Type: webrtc.TypeRTCPFBCCM, Parameter: "fir"},
				{Type: webrtc.TypeRTCPFBNACK},
				{Type: webrtc.TypeRTCPFBNACK, Parameter: "pli"},
			},
		},
	}

	return &WebRTCConfig{
		WebRTCConfig: *webRTCConfig,
		Receiver: ReceiverConfig{
			PacketBufferSizeVideo: rtcConf.PacketBufferSizeVideo,
			PacketBufferSizeAudio: rtcConf.PacketBufferSizeAudio,
		},
		Publisher:  publisherConfig,
		Subscriber: getSubscriberConfig(rtcConf.CongestionControl.UseSendSideBWEInterceptor || rtcConf.CongestionControl.UseSendSideBWE),
	}, nil
}

// UpdateCongestionControl 更新拥塞控制
func (c *WebRTCConfig) UpdateCongestionControl(ccConf config.CongestionControlConfig) {
	c.Subscriber = getSubscriberConfig(ccConf.UseSendSideBWEInterceptor || ccConf.UseSendSideBWE)
}

// SetBufferFactory 设置缓冲区工厂
func (c *WebRTCConfig) SetBufferFactory(factory *buffer.Factory) {
	c.BufferFactory = factory
	c.SettingEngine.BufferFactory = factory.GetOrNew
}

// getSubscriberConfig 获取订阅者配置
func getSubscriberConfig(enableTWCC bool) DirectionConfig {
	subscriberConfig := DirectionConfig{
		RTPHeaderExtension: RTPHeaderExtensionConfig{
			Video: []string{
				dd.ExtensionURI,
				//act.AbsCaptureTimeURI,
			},
			Audio: []string{
				//act.AbsCaptureTimeURI,
			},
		},
		RTCPFeedback: RTCPFeedbackConfig{
			Audio: []webrtc.RTCPFeedback{
				// always enable NACK for audio but disable it later for red enabled transceiver. https://github.com/pion/webrtc/pull/2972
				{Type: webrtc.TypeRTCPFBNACK},
			},
			Video: []webrtc.RTCPFeedback{
				{Type: webrtc.TypeRTCPFBCCM, Parameter: "fir"},
				{Type: webrtc.TypeRTCPFBNACK},
				{Type: webrtc.TypeRTCPFBNACK, Parameter: "pli"},
			},
		},
	}
	if enableTWCC {
		subscriberConfig.RTPHeaderExtension.Video = append(subscriberConfig.RTPHeaderExtension.Video, sdp.TransportCCURI)
		subscriberConfig.RTCPFeedback.Video = append(subscriberConfig.RTCPFeedback.Video, webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC})
	} else {
		subscriberConfig.RTPHeaderExtension.Video = append(subscriberConfig.RTPHeaderExtension.Video, sdp.ABSSendTimeURI)
		subscriberConfig.RTCPFeedback.Video = append(subscriberConfig.RTCPFeedback.Video, webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBGoogREMB})
	}

	return subscriberConfig
}
