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

package dynacast

import (
	"sync"
	"time"

	"github.com/livekit/livekit-server/pkg/sfu/mime"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
)

const (
	initialQualityUpdateWait = 10 * time.Second
)

// DynacastQualityParams 动态转码质量参数
type DynacastQualityParams struct {
	MimeType mime.MimeType
	Logger   logger.Logger
}

// DynacastQuality manages max subscribed quality of a single receiver of a media track
// 动态转码质量管理最大订阅质量的单个接收者
type DynacastQuality struct {
	params DynacastQualityParams // 参数

	// quality level enable/disable
	lock                         sync.RWMutex
	initialized                  bool                                                                    // 初始化
	maxSubscriberQuality         map[livekit.ParticipantID]livekit.VideoQuality                          // 最大订阅者质量
	maxSubscriberNodeQuality     map[livekit.NodeID]livekit.VideoQuality                                 // 最大订阅者节点质量
	maxSubscribedQuality         livekit.VideoQuality                                                    // 最大订阅质量
	maxQualityTimer              *time.Timer                                                             // 最大质量定时器
	regressTo                    *DynacastQuality                                                        // 退化到
	onSubscribedMaxQualityChange func(mimeType mime.MimeType, maxSubscribedQuality livekit.VideoQuality) // 订阅最大质量变化回调
}

// NewDynacastQuality 创建新的动态转码质量
func NewDynacastQuality(params DynacastQualityParams) *DynacastQuality {
	return &DynacastQuality{
		params:                   params,
		maxSubscriberQuality:     make(map[livekit.ParticipantID]livekit.VideoQuality),
		maxSubscriberNodeQuality: make(map[livekit.NodeID]livekit.VideoQuality),
	}
}

// Start 启动动态转码质量
func (d *DynacastQuality) Start() {
	d.reset()
}

// Restart 重启动态转码质量
func (d *DynacastQuality) Restart() {
	d.reset()
}

// Stop 停止动态转码质量
func (d *DynacastQuality) Stop() {
	d.stopMaxQualityTimer()
}

// OnSubscribedMaxQualityChange 设置订阅最大质量变化回调
func (d *DynacastQuality) OnSubscribedMaxQualityChange(f func(mimeType mime.MimeType, maxSubscribedQuality livekit.VideoQuality)) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.onSubscribedMaxQualityChange = f
}

// NotifySubscriberMaxQuality 通知订阅者最大质量
func (d *DynacastQuality) NotifySubscriberMaxQuality(subscriberID livekit.ParticipantID, quality livekit.VideoQuality) {
	d.params.Logger.Debugw(
		"setting subscriber max quality",
		"mime", d.params.MimeType,
		"subscriberID", subscriberID,
		"quality", quality.String(),
	)

	d.lock.Lock()
	if r := d.regressTo; r != nil {
		d.lock.Unlock()
		r.NotifySubscriberMaxQuality(subscriberID, quality)
		return
	}

	if quality == livekit.VideoQuality_OFF {
		delete(d.maxSubscriberQuality, subscriberID)
	} else {
		d.maxSubscriberQuality[subscriberID] = quality
	}
	d.lock.Unlock()

	d.updateQualityChange(false)
}

// NotifySubscriberNodeMaxQuality 通知订阅者节点最大质量
func (d *DynacastQuality) NotifySubscriberNodeMaxQuality(nodeID livekit.NodeID, quality livekit.VideoQuality) {
	d.params.Logger.Debugw(
		"setting subscriber node max quality",
		"mime", d.params.MimeType,
		"subscriberNodeID", nodeID,
		"quality", quality.String(),
	)

	d.lock.Lock()
	if r := d.regressTo; r != nil {
		// the downstream node will synthesize correct quality notify (its dynacast manager has codec regression), just ignore it
		d.lock.Unlock()
		r.params.Logger.Debugw("ignoring node quality change, regressed to another dynacast quality", "mime", d.params.MimeType)
		return
	}

	if quality == livekit.VideoQuality_OFF {
		delete(d.maxSubscriberNodeQuality, nodeID)
	} else {
		d.maxSubscriberNodeQuality[nodeID] = quality
	}
	d.lock.Unlock()

	d.updateQualityChange(false)
}

// RegressTo 退化到
func (d *DynacastQuality) RegressTo(other *DynacastQuality) {
	d.lock.Lock()
	d.regressTo = other
	maxSubscriberQuality := d.maxSubscriberQuality
	maxSubscriberNodeQuality := d.maxSubscriberNodeQuality
	d.maxSubscriberQuality = make(map[livekit.ParticipantID]livekit.VideoQuality)
	d.maxSubscriberNodeQuality = make(map[livekit.NodeID]livekit.VideoQuality)
	d.lock.Unlock()

	other.lock.Lock()
	for subID, quality := range maxSubscriberQuality {
		if otherQuality, ok := other.maxSubscriberQuality[subID]; ok {
			// no QUALITY_OFF in the map
			if quality > otherQuality {
				other.maxSubscriberQuality[subID] = quality
			}
		} else {
			other.maxSubscriberQuality[subID] = quality
		}
	}

	for nodeID, quality := range maxSubscriberNodeQuality {
		if otherQuality, ok := other.maxSubscriberNodeQuality[nodeID]; ok {
			// no QUALITY_OFF in the map
			if quality > otherQuality {
				other.maxSubscriberNodeQuality[nodeID] = quality
			}
		} else {
			other.maxSubscriberNodeQuality[nodeID] = quality
		}
	}
	other.lock.Unlock()
	other.Restart()
}

// reset 重置动态转码质量
func (d *DynacastQuality) reset() {
	d.lock.Lock()
	d.initialized = false
	d.lock.Unlock()

	d.startMaxQualityTimer()
}

// updateQualityChange 更新动态转码质量
func (d *DynacastQuality) updateQualityChange(force bool) {
	d.lock.Lock()
	maxSubscribedQuality := livekit.VideoQuality_OFF
	for _, subQuality := range d.maxSubscriberQuality {
		if maxSubscribedQuality == livekit.VideoQuality_OFF || (subQuality != livekit.VideoQuality_OFF && subQuality > maxSubscribedQuality) {
			maxSubscribedQuality = subQuality
		}
	}
	for _, nodeQuality := range d.maxSubscriberNodeQuality {
		if maxSubscribedQuality == livekit.VideoQuality_OFF || (nodeQuality != livekit.VideoQuality_OFF && nodeQuality > maxSubscribedQuality) {
			maxSubscribedQuality = nodeQuality
		}
	}

	if maxSubscribedQuality == d.maxSubscribedQuality && d.initialized && !force {
		d.lock.Unlock()
		return
	}

	d.initialized = true
	d.maxSubscribedQuality = maxSubscribedQuality
	d.params.Logger.Debugw("notifying quality change",
		"mime", d.params.MimeType,
		"maxSubscriberQuality", d.maxSubscriberQuality,
		"maxSubscriberNodeQuality", d.maxSubscriberNodeQuality,
		"maxSubscribedQuality", d.maxSubscribedQuality,
		"force", force,
	)
	onSubscribedMaxQualityChange := d.onSubscribedMaxQualityChange
	d.lock.Unlock()

	if onSubscribedMaxQualityChange != nil {
		onSubscribedMaxQualityChange(d.params.MimeType, maxSubscribedQuality)
	}
}

// startMaxQualityTimer 启动最大质量定时器
func (d *DynacastQuality) startMaxQualityTimer() {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.maxQualityTimer != nil {
		d.maxQualityTimer.Stop()
		d.maxQualityTimer = nil
	}

	d.maxQualityTimer = time.AfterFunc(initialQualityUpdateWait, func() {
		d.stopMaxQualityTimer()
		d.updateQualityChange(true)
	})
}

// stopMaxQualityTimer 停止最大质量定时器
func (d *DynacastQuality) stopMaxQualityTimer() {
	d.lock.Lock()
	defer d.lock.Unlock()

	if d.maxQualityTimer != nil {
		d.maxQualityTimer.Stop()
		d.maxQualityTimer = nil
	}
}
