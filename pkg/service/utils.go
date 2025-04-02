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

package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
)

// handleError 处理错误
// 增加方法与路径到错误日志中，打印错误信息，返回状态码和错误信息到客户端
//
// 参数:
// - w http.ResponseWriter: 响应写入器
// - r *http.Request: 请求
// - status int: 错误状态码
// - err error: 错误
// - keysAndValues ...interface{}: 可变参数
func handleError(w http.ResponseWriter, r *http.Request, status int, err error, keysAndValues ...interface{}) {
	keysAndValues = append(keysAndValues, "status", status)
	if r != nil && r.URL != nil {
		keysAndValues = append(keysAndValues, "method", r.Method, "path", r.URL.Path)
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(r.Context().Err(), context.Canceled) {
		logger.GetLogger().WithCallDepth(1).Warnw("error handling request", err, keysAndValues...)
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(err.Error()))
}

// boolValue 将字符串转换为布尔值
// 如果字符串为"1"或"true"，则返回true，否则返回false
//
// 参数:
// - s string: 字符串
func boolValue(s string) bool {
	return s == "1" || s == "true"
}

// RemoveDoubleSlashes 移除URL路径中的双斜杠 （中间件函数，在NewLivekitServer中调用）
// 如果URL路径以双斜杠开头，则移除第一个斜杠
//
// 参数:
// - w http.ResponseWriter: 响应写入器
// - r *http.Request: 请求
// - next http.HandlerFunc: 下一个处理器
func RemoveDoubleSlashes(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if strings.HasPrefix(r.URL.Path, "//") {
		r.URL.Path = r.URL.Path[1:]
	}
	next(w, r)
}

// IsValidDomain 检查域名是否有效
func IsValidDomain(domain string) bool {
	domainRegexp := regexp.MustCompile(`^(?i)[a-z0-9-]+(\.[a-z0-9-]+)+\.?$`)
	return domainRegexp.MatchString(domain)
}

// GetClientIP 获取客户端IP地址
func GetClientIP(r *http.Request) string {
	// CF proxy typically is first thing the user reaches
	// 通常是用户访问的第一个代理
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// 如果以上都没有，则使用请求的远程地址
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// 设置房间配置
func SetRoomConfiguration(createRequest *livekit.CreateRoomRequest, conf *livekit.RoomConfiguration) {
	if conf == nil {
		return
	}
	createRequest.Agents = conf.Agents                     // 代理
	createRequest.Egress = conf.Egress                     // 出口
	createRequest.EmptyTimeout = conf.EmptyTimeout         // 空闲超时
	createRequest.DepartureTimeout = conf.DepartureTimeout // 离开超时
	createRequest.MaxParticipants = conf.MaxParticipants   // 最大参与者
	createRequest.MinPlayoutDelay = conf.MinPlayoutDelay   // 最小播放延迟
	createRequest.MaxPlayoutDelay = conf.MaxPlayoutDelay   // 最大播放延迟
	createRequest.SyncStreams = conf.SyncStreams           // 同步流
}
