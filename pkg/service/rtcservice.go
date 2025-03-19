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
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ua-parser/uap-go/uaparser"
	"go.uber.org/atomic"
	"golang.org/x/exp/maps"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/pkg/routing/selector"
	"github.com/livekit/livekit-server/pkg/rtc"
	"github.com/livekit/livekit-server/pkg/telemetry"
	"github.com/livekit/livekit-server/pkg/telemetry/prometheus"
	"github.com/livekit/livekit-server/pkg/utils"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/psrpc"
)

type RTCService struct {
	router        routing.MessageRouter
	roomAllocator RoomAllocator
	store         ServiceStore
	upgrader      websocket.Upgrader
	currentNode   routing.LocalNode
	config        *config.Config
	isDev         bool
	limits        config.LimitConfig
	parser        *uaparser.Parser
	telemetry     telemetry.TelemetryService

	mu          sync.Mutex
	connections map[*websocket.Conn]struct{}
}

func NewRTCService(
	conf *config.Config,
	ra RoomAllocator,
	store ServiceStore,
	router routing.MessageRouter,
	currentNode routing.LocalNode,
	telemetry telemetry.TelemetryService,
) *RTCService {
	s := &RTCService{
		router:        router,
		roomAllocator: ra,
		store:         store,
		currentNode:   currentNode,
		config:        conf,
		isDev:         conf.Development,
		limits:        conf.Limit,
		parser:        uaparser.NewFromSaved(),
		telemetry:     telemetry,
		connections:   map[*websocket.Conn]struct{}{},
	}

	s.upgrader = websocket.Upgrader{
		EnableCompression: true,

		// allow connections from any origin, since script may be hosted anywhere
		// security is enforced by access tokens
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return s
}

func (s *RTCService) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/rtc/validate", s.validate)
}

func (s *RTCService) validate(w http.ResponseWriter, r *http.Request) {
	_, _, code, err := s.validateInternal(r)
	if err != nil {
		handleError(w, r, code, err)
		return
	}
	_, _ = w.Write([]byte("success"))
}

// 验证请求
// 返回：房间名，参与者初始化信息，HTTP状态码，错误
// 负责验证参与者的身份和权限，确保只有授权用户才能加入房间。
// 1. 获取授权信息
// 2. 确保参与者有加入房间的权限
// 3. 确保参与者身份信息有效
// 4. 确保参与者身份信息长度不超过限制
// 5. 确保房间名有效
// 6. 确保参与者有重连权限
func (s *RTCService) validateInternal(r *http.Request) (livekit.RoomName, routing.ParticipantInit, int, error) {
	claims := GetGrants(r.Context()) // 获取授权信息
	var pi routing.ParticipantInit   // 参与者初始化信息

	// 此处删除了与EnsureJoinPermission相同的逻辑  cqlmq 2025-03-18

	// 确保参与者有加入房间的权限
	onlyName, err := EnsureJoinPermission(r.Context())
	if err != nil {
		return "", pi, http.StatusUnauthorized, err
	}

	// 确保参与者身份信息有效
	if claims.Identity == "" {
		return "", pi, http.StatusBadRequest, ErrIdentityEmpty
	}

	// 确保参与者身份信息长度不超过限制
	if limit := s.config.Limit.MaxParticipantIdentityLength; limit > 0 && len(claims.Identity) > limit {
		return "", pi, http.StatusBadRequest, fmt.Errorf("%w: max length %d", ErrParticipantIdentityExceedsLimits, limit)
	}

	roomName := livekit.RoomName(r.FormValue("room"))                   // 房间名
	reconnectParam := r.FormValue("reconnect")                          // 重连参数
	reconnectReason, _ := strconv.Atoi(r.FormValue("reconnect_reason")) // 0 means unknown reason 重连原因
	autoSubParam := r.FormValue("auto_subscribe")                       // 自动订阅参数
	publishParam := r.FormValue("publish")                              // 发布参数
	adaptiveStreamParam := r.FormValue("adaptive_stream")               // 自适应流参数
	participantID := r.FormValue("sid")                                 // 参与者ID
	subscriberAllowPauseParam := r.FormValue("subscriber_allow_pause")  // 订阅允许暂停参数
	disableICELite := r.FormValue("disable_ice_lite")                   // 禁用ICE Lite参数

	// 如果只有onlyName，则使用onlyName作为房间名
	if onlyName != "" {
		roomName = onlyName
	}
	// 确保房间名长度不超过限制
	if limit := s.config.Limit.MaxRoomNameLength; limit > 0 && len(roomName) > limit {
		return "", pi, http.StatusBadRequest, fmt.Errorf("%w: max length %d", ErrRoomNameExceedsLimits, limit)
	}

	// this is new connection for existing participant -  with publish only permissions
	// 如果发布参数不为空，则确保参与者有发布权限
	if publishParam != "" {
		// Make sure grant has GetCanPublish set,
		// 确保授权有GetCanPublish设置
		if !claims.Video.GetCanPublish() {
			return "", routing.ParticipantInit{}, http.StatusUnauthorized, rtc.ErrPermissionDenied
		}
		// Make sure by default subscribe is off
		// 确保默认订阅是关闭的
		claims.Video.SetCanSubscribe(false)
		// 将发布参数添加到参与者ID中，作用？
		claims.Identity += "#" + publishParam
	}

	// room allocator validations
	// 使用房间分配器验证创建房间是否有效
	err = s.roomAllocator.ValidateCreateRoom(r.Context(), roomName)
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			return "", pi, http.StatusNotFound, err
		} else {
			return "", pi, http.StatusInternalServerError, err
		}
	}

	region := ""
	// 获取路由器区域，验证节点是否存在，是否达到限制
	if router, ok := s.router.(routing.Router); ok {
		region = router.GetRegion()
		// 获取节点
		if foundNode, err := router.GetNodeForRoom(r.Context(), roomName); err == nil {
			// 如果节点限制达到，则返回错误
			if selector.LimitsReached(s.limits, foundNode.Stats) {
				return "", pi, http.StatusServiceUnavailable, rtc.ErrLimitExceeded
			}
		}
	}

	// 创建房间请求的参数
	createRequest := &livekit.CreateRoomRequest{
		Name:       string(roomName),
		RoomPreset: claims.RoomPreset,
	}
	// 解析claims中的配置数据，并设置到createRequest中
	SetRoomConfiguration(createRequest, claims.GetRoomConfiguration())

	// 创建参与者初始化信息
	pi = routing.ParticipantInit{
		Reconnect:       boolValue(reconnectParam),
		ReconnectReason: livekit.ReconnectReason(reconnectReason),
		Identity:        livekit.ParticipantIdentity(claims.Identity),
		Name:            livekit.ParticipantName(claims.Name),
		AutoSubscribe:   true,
		Client:          s.ParseClientInfo(r),
		Grants:          claims,
		Region:          region,
		CreateRoom:      createRequest,
	}
	// 如果重连，则设置参与者ID
	if pi.Reconnect {
		pi.ID = livekit.ParticipantID(participantID)
	}

	if autoSubParam != "" {
		pi.AutoSubscribe = boolValue(autoSubParam)
	}
	// 如果自适应流参数不为空，则设置自适应流
	if adaptiveStreamParam != "" {
		pi.AdaptiveStream = boolValue(adaptiveStreamParam)
	}
	// 如果订阅允许暂停参数不为空，则设置订阅允许暂停
	if subscriberAllowPauseParam != "" {
		subscriberAllowPause := boolValue(subscriberAllowPauseParam)
		pi.SubscriberAllowPause = &subscriberAllowPause
	}
	// 如果禁用ICE Lite参数不为空，则设置禁用ICE Lite
	if disableICELite != "" {
		pi.DisableICELite = boolValue(disableICELite)
	}
	// 返回房间名，参与者初始化信息，HTTP状态码，错误
	return roomName, pi, http.StatusOK, nil
}

func (s *RTCService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// reject non websocket requests
	// 如果请求不是WebSocket升级请求，则返回404
	if !websocket.IsWebSocketUpgrade(r) {
		w.WriteHeader(404)
		return
	}

	// 验证请求
	roomName, pi, code, err := s.validateInternal(r)
	if err != nil {
		handleError(w, r, code, err)
		return
	}

	loggerFields := []any{
		"participant", pi.Identity,
		"pID", pi.ID,
		"room", roomName,
		"remote", false,
	}
	pLogger := utils.GetLogger(r.Context()).WithValues(loggerFields...)

	// give it a few attempts to start session
	// 给几次尝试来启动会话
	var cr connectionResult
	var initialResponse *livekit.SignalResponse
	for attempt := 0; attempt < s.config.SignalRelay.ConnectAttempts; attempt++ {
		connectionTimeout := 3 * time.Second * time.Duration(attempt+1)                    // 连接超时时间, 3秒, 6秒, 9秒 ..
		ctx := utils.ContextWithAttempt(r.Context(), attempt)                              // 上下文, 将尝试次数添加到上下文中
		cr, initialResponse, err = s.startConnection(ctx, roomName, pi, connectionTimeout) // 启动连接，返回连接结果，初始响应，错误
		if err == nil || errors.Is(err, context.Canceled) {                                // 如果连接成功或被取消，则退出
			break
		}
	}

	if err != nil {
		// 如果连接失败，则增加失败计数
		prometheus.IncrementParticipantJoinFail(1)
		status := http.StatusInternalServerError
		var psrpcErr psrpc.Error
		if errors.As(err, &psrpcErr) {
			status = psrpcErr.ToHttp()
		}
		handleError(w, r, status, err, loggerFields...)
		return
	}

	// 增加连接计数
	prometheus.IncrementParticipantJoin(1)

	// 如果连接不是重连，并且初始响应有加入信息，则设置参与者ID
	if !pi.Reconnect && initialResponse.GetJoin() != nil {
		pi.ID = livekit.ParticipantID(initialResponse.GetJoin().GetParticipant().GetSid())
	}

	// 创建信号统计
	signalStats := telemetry.NewBytesSignalStats(r.Context(), s.telemetry)

	// 如果初始响应有加入信息，则解析房间和参与者
	if join := initialResponse.GetJoin(); join != nil {
		signalStats.ResolveRoom(join.GetRoom())
		signalStats.ResolveParticipant(join.GetParticipant())
	}

	// 如果连接是重连，并且参与者ID不为空，则解析参与者
	if pi.Reconnect && pi.ID != "" {
		signalStats.ResolveParticipant(&livekit.ParticipantInfo{
			Sid:      string(pi.ID),
			Identity: string(pi.Identity),
		})
	}

	closedByClient := atomic.NewBool(false) // 客户端关闭连接标示
	done := make(chan struct{})             // 通道，用于关闭连接，控制子协程的退出
	// function exits when websocket terminates, it'll close the event reading off of request sink and response source as well
	// 当WebSocket终止时，函数退出，它会关闭请求源和响应源的读取事件
	defer func() {
		pLogger.Debugw("finishing WS connection",
			"connID", cr.ConnectionID,
			"closedByClient", closedByClient.Load(),
		)
		cr.ResponseSource.Close() // 关闭响应源
		cr.RequestSink.Close()    // 关闭请求源
		close(done)               // 关闭通道

		signalStats.Stop() // 停止信号统计
	}()

	// upgrade only once the basics are good to go
	// 升级WebSocket连接
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		handleError(w, r, http.StatusInternalServerError, err, loggerFields...)
		return
	}

	s.mu.Lock()
	s.connections[conn] = struct{}{} // 将连接添加到连接池
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.connections, conn) // 从连接池中删除连接
		s.mu.Unlock()
	}()

	// websocket established
	sigConn := NewWSSignalConnection(conn)               // 创建信号连接(对连接进行封装，完成心跳，及提供一些方法)
	count, err := sigConn.WriteResponse(initialResponse) // 写入初始响应
	if err != nil {
		pLogger.Warnw("could not write initial response", err)
		return
	}
	signalStats.AddBytes(uint64(count), true) // 添加字节统计（true表示写入/发送）

	// 调试日志, 到此为止，连接已经建立
	pLogger.Debugw("new client WS connected",
		"connID", cr.ConnectionID,
		"reconnect", pi.Reconnect,
		"reconnectReason", pi.ReconnectReason,
		"adaptiveStream", pi.AdaptiveStream,
		"selectedNodeID", cr.NodeID,
		"nodeSelectionReason", cr.NodeSelectionReason,
	)

	// handle responses
	// 处理响应，从响应源读取消息，并写入WebSocket
	go func() {
		defer func() {
			// when the source is terminated, this means Participant.Close had been called and RTC connection is done
			// we would terminate the signal connection as well
			// 当源终止时，这意味着Participant.Close已经调用，RTC连接完成
			// 我们也会终止信号连接
			closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
			_ = conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))
			_ = conn.Close()
		}()
		defer func() {
			// 如果发生恐慌，则退出
			if r := rtc.Recover(pLogger); r != nil {
				os.Exit(1)
			}
		}()
		for {
			select {
			case <-done: // 通道关闭，退出
				return
			case msg := <-cr.ResponseSource.ReadChan(): // 从响应源读取消息
				if msg == nil {
					pLogger.Debugw("nothing to read from response source", "connID", cr.ConnectionID)
					return
				}
				res, ok := msg.(*livekit.SignalResponse) // 断言消息类型为SignalResponse
				if !ok {
					pLogger.Errorw(
						"unexpected message type", nil,
						"type", fmt.Sprintf("%T", msg),
						"connID", cr.ConnectionID,
					)
					continue
				}

				// 处理消息，根据消息类型进行处理
				switch m := res.Message.(type) {
				case *livekit.SignalResponse_Offer: // 处理Offer消息
					pLogger.Debugw("sending offer", "offer", m)
				case *livekit.SignalResponse_Answer: // 处理Answer消息
					pLogger.Debugw("sending answer", "answer", m)
				case *livekit.SignalResponse_Join: // 处理Join消息
					pLogger.Debugw("sending join", "join", m)
					signalStats.ResolveRoom(m.Join.GetRoom())
					signalStats.ResolveParticipant(m.Join.GetParticipant())
				case *livekit.SignalResponse_RoomUpdate: // 处理RoomUpdate消息
					pLogger.Debugw("sending room update", "roomUpdate", m)
					signalStats.ResolveRoom(m.RoomUpdate.GetRoom())
				case *livekit.SignalResponse_Update: // 处理Update消息
					pLogger.Debugw("sending participant update", "participantUpdate", m)
				}

				// 写入WebSocket，并添加字节统计
				if count, err := sigConn.WriteResponse(res); err != nil {
					pLogger.Warnw("error writing to websocket", err)
					return
				} else {
					signalStats.AddBytes(uint64(count), true)
				}
			}
		}
	}()

	// handle incoming requests from websocket
	// 从WebSocket读取请求，并写入请求源
	for {
		req, count, err := sigConn.ReadRequest()
		if err != nil {
			if IsWebSocketCloseError(err) {
				closedByClient.Store(true)
			} else {
				pLogger.Errorw("error reading from websocket", err, "connID", cr.ConnectionID)
			}
			return
		}
		signalStats.AddBytes(uint64(count), false) // 添加字节统计（false表示读取/接收）

		switch m := req.Message.(type) {
		case *livekit.SignalRequest_Ping:
			// 写入Pong响应
			count, perr := sigConn.WriteResponse(&livekit.SignalResponse{
				Message: &livekit.SignalResponse_Pong{
					//
					// Although this field is int64, some clients (like JS) cause overflow if nanosecond granularity is used.
					// 虽然这个字段是int64，但一些客户端（如JS）在纳秒粒度下会导致溢出。
					// So. use UnixMillis().
					// 所以，使用UnixMillis()。
					// 作用：返回当前时间戳，对方收到后，可以计算出Pong延迟
					Pong: time.Now().UnixMilli(), // 毫秒，防止溢出
				},
			})
			if perr == nil {
				signalStats.AddBytes(uint64(count), true)
			}
		case *livekit.SignalRequest_PingReq:
			// 写入PongResp响应
			count, perr := sigConn.WriteResponse(&livekit.SignalResponse{
				Message: &livekit.SignalResponse_PongResp{
					PongResp: &livekit.Pong{
						LastPingTimestamp: m.PingReq.Timestamp,    // 作用：返回最后一次Ping的时间戳，对方收到后，可以计算出Ping延迟
						Timestamp:         time.Now().UnixMilli(), // 作用：返回当前时间戳，对方收到后，可以计算出Pong延迟
					},
				},
			})
			if perr == nil {
				signalStats.AddBytes(uint64(count), true)
			}
		}

		switch m := req.Message.(type) {
		case *livekit.SignalRequest_Offer:
			pLogger.Debugw("received offer", "offer", m)
		case *livekit.SignalRequest_Answer:
			pLogger.Debugw("received answer", "answer", m)
		}

		// 将消息写入请求源（进行实际的业务处理？）
		if err := cr.RequestSink.WriteMessage(req); err != nil {
			pLogger.Warnw("error writing to request sink", err, "connID", cr.ConnectionID)
			return
		}
	}
}

// 客户端类型映射
// 以后优化时可以考虑直接使用livekit.ClientInfo_SDK_name
// livekit.ClientInfo_SDK_name 是livekit.ClientInfo_SDK的枚举值 , 如果这样，建议客户端口使用大写并与livekit.ClientInfo_SDK_name一致
var sdkMap = map[string]livekit.ClientInfo_SDK{
	"js":          livekit.ClientInfo_JS,
	"ios":         livekit.ClientInfo_SWIFT,
	"swift":       livekit.ClientInfo_SWIFT,
	"android":     livekit.ClientInfo_ANDROID,
	"flutter":     livekit.ClientInfo_FLUTTER,
	"go":          livekit.ClientInfo_GO,
	"unity":       livekit.ClientInfo_UNITY,
	"reactnative": livekit.ClientInfo_REACT_NATIVE,
	"rust":        livekit.ClientInfo_RUST,
	"python":      livekit.ClientInfo_PYTHON,
	"cpp":         livekit.ClientInfo_CPP,
	"unityweb":    livekit.ClientInfo_UNITY_WEB,
	"node":        livekit.ClientInfo_NODE,
}

// 解析客户端信息
func (s *RTCService) ParseClientInfo(r *http.Request) *livekit.ClientInfo {
	values := r.Form
	ci := &livekit.ClientInfo{} // 初始化客户端信息
	if pv, err := strconv.Atoi(values.Get("protocol")); err == nil {
		ci.Protocol = int32(pv) // 协议，可选项？：1-JSON, 2-Protobuf
	}

	// 解析客户端类型
	sdkString := values.Get("sdk") // 客户端类型
	if sdk, ok := sdkMap[sdkString]; ok {
		ci.Sdk = sdk
	}

	ci.Version = values.Get("version")
	ci.Os = values.Get("os")
	ci.OsVersion = values.Get("os_version")
	ci.Browser = values.Get("browser")
	ci.BrowserVersion = values.Get("browser_version")
	ci.DeviceModel = values.Get("device_model")
	ci.Network = values.Get("network")
	ci.Address = GetClientIP(r)

	// attempt to parse types for SDKs that support browser as a platform
	// 尝试解析支持浏览器作为平台的SDK类型
	if ci.Sdk == livekit.ClientInfo_JS ||
		ci.Sdk == livekit.ClientInfo_REACT_NATIVE ||
		ci.Sdk == livekit.ClientInfo_FLUTTER ||
		ci.Sdk == livekit.ClientInfo_UNITY {
		client := s.parser.Parse(r.UserAgent())
		if ci.Browser == "" {
			ci.Browser = client.UserAgent.Family
			ci.BrowserVersion = client.UserAgent.ToVersionString()
		}
		if ci.Os == "" {
			ci.Os = client.Os.Family
			ci.OsVersion = client.Os.ToVersionString()
		}
		if ci.DeviceModel == "" {
			model := client.Device.Family
			if model != "" && client.Device.Model != "" && model != client.Device.Model {
				model += " " + client.Device.Model
			}

			ci.DeviceModel = model
		}
	}

	return ci
}

// 定期关闭连接
func (s *RTCService) DrainConnections(interval time.Duration) {
	s.mu.Lock()
	conns := maps.Clone(s.connections)
	s.mu.Unlock()

	// jitter drain start
	time.Sleep(time.Duration(rand.Int63n(int64(interval))))

	t := time.NewTicker(interval)
	defer t.Stop()

	for c := range conns {
		_ = c.Close()
		<-t.C
	}
}

// 连接结果
type connectionResult struct {
	routing.StartParticipantSignalResults
	Room *livekit.Room
}

// 启动连接
func (s *RTCService) startConnection(
	ctx context.Context,
	roomName livekit.RoomName,
	pi routing.ParticipantInit,
	timeout time.Duration,
) (connectionResult, *livekit.SignalResponse, error) {
	var cr connectionResult
	var err error

	// 选择房间节点（如果没有问题，房间节点会自动分配到内部的管理数据中）
	if err := s.roomAllocator.SelectRoomNode(ctx, roomName, ""); err != nil {
		return cr, nil, err
	}

	// this needs to be started first *before* using router functions on this node
	// 启动参与者信号（如果成功，则返回参与者信号结果）
	// 这里是媒体服务器介入的关键点
	cr.StartParticipantSignalResults, err = s.router.StartParticipantSignal(ctx, roomName, pi)
	if err != nil {
		return cr, nil, err
	}

	// wait for the first message before upgrading to websocket. If no one is
	// responding to our connection attempt, we should terminate the connection
	// instead of waiting forever on the WebSocket
	// 在升级到WebSocket之前等待第一条消息。如果没有人响应我们的连接尝试，
	// 我们应该终止连接，而不是无限期地等待WebSocket
	// 这个消息可能是媒体服务器发送的？
	initialResponse, err := readInitialResponse(cr.ResponseSource, timeout)
	if err != nil {
		// close the connection to avoid leaking
		cr.RequestSink.Close()
		cr.ResponseSource.Close()
		return cr, nil, err
	}

	return cr, initialResponse, nil
}

// 读取初始响应
func readInitialResponse(source routing.MessageSource, timeout time.Duration) (*livekit.SignalResponse, error) {
	responseTimer := time.NewTimer(timeout)
	defer responseTimer.Stop()
	for {
		select {
		case <-responseTimer.C:
			return nil, errors.New("timed out while waiting for signal response")
		case msg := <-source.ReadChan():
			if msg == nil {
				return nil, errors.New("connection closed by media")
			}
			res, ok := msg.(*livekit.SignalResponse)
			if !ok {
				return nil, fmt.Errorf("unexpected message type: %T", msg)
			}
			return res, nil
		}
	}
}
