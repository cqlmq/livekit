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
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	goversion "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"

	"github.com/livekit/protocol/ingress"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	"github.com/livekit/protocol/utils"
	"github.com/livekit/protocol/utils/guid"
	"github.com/livekit/psrpc"

	"github.com/livekit/livekit-server/version"
)

const (
	VersionKey = "livekit_version"

	// RoomsKey is hash of room_name => Room proto
	RoomsKey        = "rooms"
	RoomInternalKey = "room_internal"

	// EgressKey is a hash of egressID => egress info
	EgressKey        = "egress"
	EndedEgressKey   = "ended_egress"
	RoomEgressPrefix = "egress:room:"

	// IngressKey is a hash of ingressID => ingress info
	IngressKey         = "ingress"
	StreamKeyKey       = "{ingress}_stream_key"
	IngressStatePrefix = "{ingress}_state:"
	RoomIngressPrefix  = "room_{ingress}:"

	// RoomParticipantsPrefix is hash of participant_name => ParticipantInfo
	RoomParticipantsPrefix = "room_participants:"

	// RoomLockPrefix is a simple key containing a provided lock uid
	RoomLockPrefix = "room_lock:"

	// Agents
	AgentDispatchPrefix = "agent_dispatch:"
	AgentJobPrefix      = "agent_job:"

	maxRetries = 5
)

// RedisStore 是Redis存储的实现
// 它实现了ObjectStore接口, 包括继承的ServiceStore接口
// 它还实现了SIPStore接口
// 它还实现了AgentStore接口
// 它还实现了IngressStore接口
// 它还实现了EgressStore接口
type RedisStore struct {
	rc           redis.UniversalClient
	unlockScript *redis.Script
	ctx          context.Context
	done         chan struct{}
}

func NewRedisStore(rc redis.UniversalClient) *RedisStore {
	// 可以理解为：如果Redis中存在该key，则删除该key，否则返回0，原子操作，在分布式环境下，可以保证安全
	unlockScript := `if redis.call("get", KEYS[1]) == ARGV[1] then
						return redis.call("del", KEYS[1])
					 else return 0
					 end`

	return &RedisStore{
		ctx:          context.Background(),
		rc:           rc,
		unlockScript: redis.NewScript(unlockScript),
	}
}

// Start 启动RedisStore
func (s *RedisStore) Start() error {
	if s.done != nil {
		return nil
	}

	s.done = make(chan struct{}, 1)

	// 获取Redis中的版本号
	v, err := s.rc.Get(s.ctx, VersionKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	if v == "" {
		v = "0.0.0"
	}
	existing, _ := goversion.NewVersion(v)
	current, _ := goversion.NewVersion(version.Version)
	// 如果当前版本号大于Redis中的版本号，则设置为当前版本号
	if current.GreaterThan(existing) {
		if err = s.rc.Set(s.ctx, VersionKey, version.Version, 0).Err(); err != nil {
			return err
		}
	} else if current.LessThan(existing) {
		// 如果当前版本号小于Redis中的版本号，打印一个信息, 提示用户升级
		logger.Warnw("current version is less than redis version", nil, "current", current, "redis", existing)
	}

	go s.egressWorker() // 启动一个协程，用于处理Egress
	return nil
}

// Stop 停止RedisStore
func (s *RedisStore) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

// StoreRoom 存储房间
// 实现了ObjectStore接口
func (s *RedisStore) StoreRoom(_ context.Context, room *livekit.Room, internal *livekit.RoomInternal) error {
	if room.CreationTime == 0 {
		now := time.Now()
		room.CreationTime = now.Unix()
		room.CreationTimeMs = now.UnixMilli()
	}

	// 将Room对象序列化为字节切片
	roomData, err := proto.Marshal(room)
	if err != nil {
		return err
	}

	// 使用Pipeline批量执行Redis操作
	pp := s.rc.Pipeline()
	// 将Room对象存储到Redis哈希表中
	pp.HSet(s.ctx, RoomsKey, room.Name, roomData)

	// 如果存在RoomInternal对象，则将其序列化并存储到Redis哈希表中
	if internal != nil {
		internalData, err := proto.Marshal(internal)
		if err != nil {
			return err
		}
		pp.HSet(s.ctx, RoomInternalKey, room.Name, internalData)
	} else {
		pp.HDel(s.ctx, RoomInternalKey, room.Name)
	}

	if _, err = pp.Exec(s.ctx); err != nil {
		return errors.Wrap(err, "could not create room")
	}
	return nil
}

// LoadRoom 加载房间
// 实现了ObjectStore->ServiceStore接口
func (s *RedisStore) LoadRoom(_ context.Context, roomName livekit.RoomName, includeInternal bool) (*livekit.Room, *livekit.RoomInternal, error) {
	pp := s.rc.Pipeline()
	pp.HGet(s.ctx, RoomsKey, string(roomName))
	if includeInternal {
		pp.HGet(s.ctx, RoomInternalKey, string(roomName))
	}

	res, err := pp.Exec(s.ctx)
	if err != nil && err != redis.Nil {
		// if the room exists but internal does not, the pipeline will still return redis.Nil
		return nil, nil, err
	}

	room := &livekit.Room{}
	roomData, err := res[0].(*redis.StringCmd).Result()
	if err != nil {
		if err == redis.Nil {
			err = ErrRoomNotFound
		}
		return nil, nil, err
	}
	if err = proto.Unmarshal([]byte(roomData), room); err != nil {
		return nil, nil, err
	}

	var internal *livekit.RoomInternal
	if includeInternal {
		internalData, err := res[1].(*redis.StringCmd).Result()
		if err == nil {
			internal = &livekit.RoomInternal{}
			if err = proto.Unmarshal([]byte(internalData), internal); err != nil {
				return nil, nil, err
			}
		} else if err != redis.Nil {
			return nil, nil, err
		}
	}

	return room, internal, nil
}

// ListRooms 列出房间
// 实现了ServiceStore接口
func (s *RedisStore) ListRooms(_ context.Context, roomNames []livekit.RoomName) ([]*livekit.Room, error) {
	var items []string
	var err error
	if roomNames == nil {
		items, err = s.rc.HVals(s.ctx, RoomsKey).Result()
		if err != nil && err != redis.Nil {
			return nil, errors.Wrap(err, "could not get rooms")
		}
	} else {
		names := livekit.IDsAsStrings(roomNames)
		var results []interface{}
		results, err = s.rc.HMGet(s.ctx, RoomsKey, names...).Result()
		if err != nil && err != redis.Nil {
			return nil, errors.Wrap(err, "could not get rooms by names")
		}
		for _, r := range results {
			if item, ok := r.(string); ok {
				items = append(items, item)
			}
		}
	}

	rooms := make([]*livekit.Room, 0, len(items))

	for _, item := range items {
		room := livekit.Room{}
		err := proto.Unmarshal([]byte(item), &room)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, &room)
	}
	return rooms, nil
}

// DeleteRoom 删除房间
// 实现了ObjectStore->ServiceStore接口
func (s *RedisStore) DeleteRoom(ctx context.Context, roomName livekit.RoomName) error {
	_, _, err := s.LoadRoom(ctx, roomName, false)
	if err == ErrRoomNotFound {
		return nil
	}

	pp := s.rc.Pipeline()
	pp.HDel(s.ctx, RoomsKey, string(roomName))
	pp.HDel(s.ctx, RoomInternalKey, string(roomName))
	pp.Del(s.ctx, RoomParticipantsPrefix+string(roomName))
	pp.Del(s.ctx, AgentDispatchPrefix+string(roomName))
	pp.Del(s.ctx, AgentJobPrefix+string(roomName))

	_, err = pp.Exec(s.ctx)
	return err
}

// LockRoom 锁定房间
// 实现了ObjectStore接口
func (s *RedisStore) LockRoom(_ context.Context, roomName livekit.RoomName, duration time.Duration) (string, error) {
	token := guid.New("LOCK")
	key := RoomLockPrefix + string(roomName)

	startTime := time.Now()
	for {
		locked, err := s.rc.SetNX(s.ctx, key, token, duration).Result()
		if err != nil {
			return "", err
		}
		if locked {
			return token, nil
		}

		// stop waiting past lock duration
		if time.Since(startTime) > duration {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	return "", ErrRoomLockFailed
}

// UnlockRoom 解锁房间
// 实现了ObjectStore接口
func (s *RedisStore) UnlockRoom(_ context.Context, roomName livekit.RoomName, uid string) error {
	key := RoomLockPrefix + string(roomName)
	res, err := s.unlockScript.Run(s.ctx, s.rc, []string{key}, uid).Result()
	if err != nil {
		return err
	}

	// uid does not match
	if i, ok := res.(int64); !ok || i != 1 {
		return ErrRoomUnlockFailed
	}

	return nil
}

// StoreParticipant 存储参与者
// 实现了ObjectStore接口
func (s *RedisStore) StoreParticipant(_ context.Context, roomName livekit.RoomName, participant *livekit.ParticipantInfo) error {
	key := RoomParticipantsPrefix + string(roomName)

	data, err := proto.Marshal(participant)
	if err != nil {
		return err
	}

	return s.rc.HSet(s.ctx, key, participant.Identity, data).Err()
}

// LoadParticipant 加载参与者
// 实现了ServiceStore接口
func (s *RedisStore) LoadParticipant(_ context.Context, roomName livekit.RoomName, identity livekit.ParticipantIdentity) (*livekit.ParticipantInfo, error) {
	key := RoomParticipantsPrefix + string(roomName)
	data, err := s.rc.HGet(s.ctx, key, string(identity)).Result()
	if err == redis.Nil {
		return nil, ErrParticipantNotFound
	} else if err != nil {
		return nil, err
	}

	pi := livekit.ParticipantInfo{}
	if err := proto.Unmarshal([]byte(data), &pi); err != nil {
		return nil, err
	}
	return &pi, nil
}

// ListParticipants 列出参与者
// 实现了ServiceStore接口
func (s *RedisStore) ListParticipants(_ context.Context, roomName livekit.RoomName) ([]*livekit.ParticipantInfo, error) {
	key := RoomParticipantsPrefix + string(roomName)
	items, err := s.rc.HVals(s.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	participants := make([]*livekit.ParticipantInfo, 0, len(items))
	for _, item := range items {
		pi := livekit.ParticipantInfo{}
		if err := proto.Unmarshal([]byte(item), &pi); err != nil {
			return nil, err
		}
		participants = append(participants, &pi)
	}
	return participants, nil
}

// DeleteParticipant 删除参与者
// 实现了ObjectStore接口
func (s *RedisStore) DeleteParticipant(_ context.Context, roomName livekit.RoomName, identity livekit.ParticipantIdentity) error {
	key := RoomParticipantsPrefix + string(roomName)

	return s.rc.HDel(s.ctx, key, string(identity)).Err()
}

// StoreEgress 存储Egress
// 实现了EgressStore接口
func (s *RedisStore) StoreEgress(_ context.Context, info *livekit.EgressInfo) error {
	data, err := proto.Marshal(info)
	if err != nil {
		return err
	}

	pp := s.rc.Pipeline()
	pp.HSet(s.ctx, EgressKey, info.EgressId, data)
	pp.SAdd(s.ctx, RoomEgressPrefix+info.RoomName, info.EgressId)
	if _, err = pp.Exec(s.ctx); err != nil {
		return errors.Wrap(err, "could not store egress info")
	}

	return nil
}

// LoadEgress 加载Egress
// 实现了EgressStore接口
func (s *RedisStore) LoadEgress(_ context.Context, egressID string) (*livekit.EgressInfo, error) {
	data, err := s.rc.HGet(s.ctx, EgressKey, egressID).Result()
	switch err {
	case nil:
		info := &livekit.EgressInfo{}
		err = proto.Unmarshal([]byte(data), info)
		if err != nil {
			return nil, err
		}
		return info, nil

	case redis.Nil:
		return nil, ErrEgressNotFound

	default:
		return nil, err
	}
}

// ListEgress 列出Egress
// 实现了EgressStore接口
func (s *RedisStore) ListEgress(_ context.Context, roomName livekit.RoomName, active bool) ([]*livekit.EgressInfo, error) {
	var infos []*livekit.EgressInfo

	if roomName == "" {
		data, err := s.rc.HGetAll(s.ctx, EgressKey).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, nil
			}
			return nil, err
		}

		for _, d := range data {
			info := &livekit.EgressInfo{}
			err = proto.Unmarshal([]byte(d), info)
			if err != nil {
				return nil, err
			}

			// if active, filter status starting, active, and ending
			if !active || int32(info.Status) < int32(livekit.EgressStatus_EGRESS_COMPLETE) {
				infos = append(infos, info)
			}
		}
	} else {
		egressIDs, err := s.rc.SMembers(s.ctx, RoomEgressPrefix+string(roomName)).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, nil
			}
			return nil, err
		}

		data, _ := s.rc.HMGet(s.ctx, EgressKey, egressIDs...).Result()
		for _, d := range data {
			if d == nil {
				continue
			}
			info := &livekit.EgressInfo{}
			err = proto.Unmarshal([]byte(d.(string)), info)
			if err != nil {
				return nil, err
			}

			// if active, filter status starting, active, and ending
			if !active || int32(info.Status) < int32(livekit.EgressStatus_EGRESS_COMPLETE) {
				infos = append(infos, info)
			}
		}
	}

	return infos, nil
}

// UpdateEgress 更新Egress
// 实现了EgressStore接口
func (s *RedisStore) UpdateEgress(_ context.Context, info *livekit.EgressInfo) error {
	data, err := proto.Marshal(info)
	if err != nil {
		return err
	}

	if info.EndedAt != 0 {
		pp := s.rc.Pipeline()
		pp.HSet(s.ctx, EgressKey, info.EgressId, data)
		pp.HSet(s.ctx, EndedEgressKey, info.EgressId, egressEndedValue(info.RoomName, info.EndedAt))
		_, err = pp.Exec(s.ctx)
	} else {
		err = s.rc.HSet(s.ctx, EgressKey, info.EgressId, data).Err()
	}

	if err != nil {
		return errors.Wrap(err, "could not update egress info")
	}

	return nil
}

// Deletes egress info 24h after the egress has ended
// 定期清理结束超过24小时的Egress信息
func (s *RedisStore) egressWorker() {
	ticker := time.NewTicker(time.Minute * 30)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			err := s.CleanEndedEgress()
			if err != nil {
				logger.Errorw("could not clean egress info", err)
			}
		}
	}
}

// CleanEndedEgress 清理结束超过24小时的Egress信息
func (s *RedisStore) CleanEndedEgress() error {
	values, err := s.rc.HGetAll(s.ctx, EndedEgressKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}

	expiry := time.Now().Add(-24 * time.Hour).UnixNano()
	for egressID, val := range values {
		roomName, endedAt, err := parseEgressEnded(val)
		if err != nil {
			return err
		}

		if endedAt < expiry {
			pp := s.rc.Pipeline()
			pp.SRem(s.ctx, RoomEgressPrefix+roomName, egressID)
			pp.HDel(s.ctx, EgressKey, egressID)
			// Delete the EndedEgressKey entry last so that future sweeper runs get another chance to delete dangling data is the deletion partially failed.
			pp.HDel(s.ctx, EndedEgressKey, egressID)
			if _, err := pp.Exec(s.ctx); err != nil {
				return err
			}
		}
	}

	return nil
}

// egressEndedValue 返回房间名称和结束时间的格式化字符串，格式为"roomName|endedAt"
func egressEndedValue(roomName string, endedAt int64) string {
	return fmt.Sprintf("%s|%d", roomName, endedAt)
}

// parseEgressEnded 解析存储的Egress结束信息，返回房间名称和结束时间
func parseEgressEnded(value string) (roomName string, endedAt int64, err error) {
	s := strings.Split(value, "|")
	if len(s) != 2 {
		err = errors.New("invalid egressEnded value")
		return
	}

	roomName = s[0]
	endedAt, err = strconv.ParseInt(s[1], 10, 64)
	return
}

// StoreIngress 存储Ingress
// 实现了IngressStore接口
func (s *RedisStore) StoreIngress(ctx context.Context, info *livekit.IngressInfo) error {
	err := s.storeIngress(ctx, info)
	if err != nil {
		return err
	}

	return s.storeIngressState(ctx, info.IngressId, nil)
}

// storeIngress 存储Ingress
func (s *RedisStore) storeIngress(_ context.Context, info *livekit.IngressInfo) error {
	if info.IngressId == "" {
		return errors.New("Missing IngressId")
	}
	if info.StreamKey == "" && info.InputType != livekit.IngressInput_URL_INPUT {
		return errors.New("Missing StreamKey")
	}

	// ignore state
	infoCopy := utils.CloneProto(info)
	infoCopy.State = nil

	data, err := proto.Marshal(infoCopy)
	if err != nil {
		return err
	}

	// Use a "transaction" to remove the old room association if it changed
	txf := func(tx *redis.Tx) error {
		var oldRoom string

		oldInfo, err := s.loadIngress(tx, info.IngressId)
		switch err {
		case ErrIngressNotFound:
			// Ingress doesn't exist yet
		case nil:
			oldRoom = oldInfo.RoomName
		default:
			return err
		}

		results, err := tx.TxPipelined(s.ctx, func(p redis.Pipeliner) error {
			p.HSet(s.ctx, IngressKey, info.IngressId, data)
			if info.StreamKey != "" {
				p.HSet(s.ctx, StreamKeyKey, info.StreamKey, info.IngressId)
			}

			if oldRoom != info.RoomName {
				if oldRoom != "" {
					p.SRem(s.ctx, RoomIngressPrefix+oldRoom, info.IngressId)
				}
				if info.RoomName != "" {
					p.SAdd(s.ctx, RoomIngressPrefix+info.RoomName, info.IngressId)
				}
			}

			return nil
		})

		if err != nil {
			return err
		}

		for _, res := range results {
			if err := res.Err(); err != nil {
				return err
			}
		}

		return nil
	}

	// Retry if the key has been changed.
	for i := 0; i < maxRetries; i++ {
		err := s.rc.Watch(s.ctx, txf, IngressKey)
		switch err {
		case redis.TxFailedErr:
			// Optimistic lock lost. Retry.
			continue
		default:
			return err
		}
	}

	return nil
}

// storeIngressState 存储Ingress状态
func (s *RedisStore) storeIngressState(_ context.Context, ingressId string, state *livekit.IngressState) error {
	if ingressId == "" {
		return errors.New("Missing IngressId")
	}

	if state == nil {
		state = &livekit.IngressState{}
	}

	data, err := proto.Marshal(state)
	if err != nil {
		return err
	}

	// Use a "transaction" to remove the old room association if it changed
	txf := func(tx *redis.Tx) error {
		var oldStartedAt int64
		var oldUpdatedAt int64

		oldState, err := s.loadIngressState(tx, ingressId)
		switch err {
		case ErrIngressNotFound:
			// Ingress state doesn't exist yet
		case nil:
			oldStartedAt = oldState.StartedAt
			oldUpdatedAt = oldState.UpdatedAt
		default:
			return err
		}

		results, err := tx.TxPipelined(s.ctx, func(p redis.Pipeliner) error {
			if state.StartedAt < oldStartedAt {
				// Do not overwrite the info and state of a more recent session
				return ingress.ErrIngressOutOfDate
			}

			if state.StartedAt == oldStartedAt && state.UpdatedAt < oldUpdatedAt {
				// Do not overwrite with an old state in case RPCs were delivered out of order.
				// All RPCs come from the same ingress server and should thus be on the same clock.
				return nil
			}

			p.Set(s.ctx, IngressStatePrefix+ingressId, data, 0)

			return nil
		})

		if err != nil {
			return err
		}

		for _, res := range results {
			if err := res.Err(); err != nil {
				return err
			}
		}

		return nil
	}

	// Retry if the key has been changed.
	for i := 0; i < maxRetries; i++ {
		err := s.rc.Watch(s.ctx, txf, IngressStatePrefix+ingressId)
		switch err {
		case redis.TxFailedErr:
			// Optimistic lock lost. Retry.
			continue
		default:
			return err
		}
	}

	return nil
}

// loadIngress 加载Ingress
func (s *RedisStore) loadIngress(c redis.Cmdable, ingressId string) (*livekit.IngressInfo, error) {
	data, err := c.HGet(s.ctx, IngressKey, ingressId).Result()
	switch err {
	case nil:
		info := &livekit.IngressInfo{}
		err = proto.Unmarshal([]byte(data), info)
		if err != nil {
			return nil, err
		}
		return info, nil

	case redis.Nil:
		return nil, ErrIngressNotFound

	default:
		return nil, err
	}
}

// loadIngressState 加载Ingress状态
func (s *RedisStore) loadIngressState(c redis.Cmdable, ingressId string) (*livekit.IngressState, error) {
	data, err := c.Get(s.ctx, IngressStatePrefix+ingressId).Result()
	switch err {
	case nil:
		state := &livekit.IngressState{}
		err = proto.Unmarshal([]byte(data), state)
		if err != nil {
			return nil, err
		}
		return state, nil

	case redis.Nil:
		return nil, ErrIngressNotFound

	default:
		return nil, err
	}
}

// LoadIngress 加载Ingress
// 实现了IngressStore接口
func (s *RedisStore) LoadIngress(_ context.Context, ingressId string) (*livekit.IngressInfo, error) {
	info, err := s.loadIngress(s.rc, ingressId)
	if err != nil {
		return nil, err
	}
	state, err := s.loadIngressState(s.rc, ingressId)
	switch err {
	case nil:
		info.State = state
	case ErrIngressNotFound:
		// No state for this ingress
	default:
		return nil, err
	}

	return info, nil
}

// LoadIngressFromStreamKey 通过流密钥加载Ingress信息
// 实现了IngressStore接口
func (s *RedisStore) LoadIngressFromStreamKey(_ context.Context, streamKey string) (*livekit.IngressInfo, error) {
	ingressID, err := s.rc.HGet(s.ctx, StreamKeyKey, streamKey).Result()
	switch err {
	case nil:
		return s.LoadIngress(s.ctx, ingressID)

	case redis.Nil:
		return nil, ErrIngressNotFound

	default:
		return nil, err
	}
}

// ListIngress 列出Ingress
// 实现了IngressStore接口
func (s *RedisStore) ListIngress(_ context.Context, roomName livekit.RoomName) ([]*livekit.IngressInfo, error) {
	var infos []*livekit.IngressInfo

	if roomName == "" {
		data, err := s.rc.HGetAll(s.ctx, IngressKey).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, nil
			}
			return nil, err
		}

		for _, d := range data {
			info := &livekit.IngressInfo{}
			err = proto.Unmarshal([]byte(d), info)
			if err != nil {
				return nil, err
			}
			state, err := s.loadIngressState(s.rc, info.IngressId)
			switch err {
			case nil:
				info.State = state
			case ErrIngressNotFound:
				// No state for this ingress
			default:
				return nil, err
			}

			infos = append(infos, info)
		}
	} else {
		ingressIDs, err := s.rc.SMembers(s.ctx, RoomIngressPrefix+string(roomName)).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, nil
			}
			return nil, err
		}

		data, _ := s.rc.HMGet(s.ctx, IngressKey, ingressIDs...).Result()
		for _, d := range data {
			if d == nil {
				continue
			}
			info := &livekit.IngressInfo{}
			err = proto.Unmarshal([]byte(d.(string)), info)
			if err != nil {
				return nil, err
			}
			state, err := s.loadIngressState(s.rc, info.IngressId)
			switch err {
			case nil:
				info.State = state
			case ErrIngressNotFound:
				// No state for this ingress
			default:
				return nil, err
			}

			infos = append(infos, info)
		}
	}

	return infos, nil
}

// UpdateIngress 更新Ingress
// 实现了IngressStore接口
func (s *RedisStore) UpdateIngress(ctx context.Context, info *livekit.IngressInfo) error {
	return s.storeIngress(ctx, info)
}

// UpdateIngressState 更新Ingress状态
// 实现了IngressStore接口
func (s *RedisStore) UpdateIngressState(ctx context.Context, ingressId string, state *livekit.IngressState) error {
	return s.storeIngressState(ctx, ingressId, state)
}

// DeleteIngress 删除Ingress
// 实现了IngressStore接口
func (s *RedisStore) DeleteIngress(_ context.Context, info *livekit.IngressInfo) error {
	tx := s.rc.TxPipeline()
	tx.SRem(s.ctx, RoomIngressPrefix+info.RoomName, info.IngressId)
	if info.StreamKey != "" {
		tx.HDel(s.ctx, StreamKeyKey, info.StreamKey)
	}
	tx.HDel(s.ctx, IngressKey, info.IngressId)
	tx.Del(s.ctx, IngressStatePrefix+info.IngressId)
	if _, err := tx.Exec(s.ctx); err != nil {
		return errors.Wrap(err, "could not delete ingress info")
	}

	return nil
}

// StoreAgentDispatch 存储AgentDispatch
// 实现了AgentStore接口
func (s *RedisStore) StoreAgentDispatch(_ context.Context, dispatch *livekit.AgentDispatch) error {
	di := utils.CloneProto(dispatch)

	// Do not store jobs with the dispatch
	if di.State != nil {
		di.State.Jobs = nil
	}

	key := AgentDispatchPrefix + string(dispatch.Room)

	data, err := proto.Marshal(di)
	if err != nil {
		return err
	}

	return s.rc.HSet(s.ctx, key, di.Id, data).Err()
}

// This will not delete the jobs created by the dispatch
// 删除AgentDispatch
// 实现了AgentStore接口
func (s *RedisStore) DeleteAgentDispatch(_ context.Context, dispatch *livekit.AgentDispatch) error {
	key := AgentDispatchPrefix + string(dispatch.Room)
	return s.rc.HDel(s.ctx, key, dispatch.Id).Err()
}

// ListAgentDispatches 列出AgentDispatch
// 实现了AgentStore接口
func (s *RedisStore) ListAgentDispatches(_ context.Context, roomName livekit.RoomName) ([]*livekit.AgentDispatch, error) {
	key := AgentDispatchPrefix + string(roomName)
	dispatches, err := redisLoadAll[livekit.AgentDispatch](s.ctx, s, key)
	if err != nil {
		return nil, err
	}

	dMap := make(map[string]*livekit.AgentDispatch)
	for _, di := range dispatches {
		dMap[di.Id] = di
	}

	key = AgentJobPrefix + string(roomName)
	jobs, err := redisLoadAll[livekit.Job](s.ctx, s, key)
	if err != nil {
		return nil, err
	}

	// Associate job to dispatch
	for _, jb := range jobs {
		di := dMap[jb.DispatchId]
		if di == nil {
			continue
		}
		if di.State == nil {
			di.State = &livekit.AgentDispatchState{}
		}
		di.State.Jobs = append(di.State.Jobs, jb)
	}

	return dispatches, nil
}

// StoreAgentJob 存储AgentJob
// 实现了AgentStore接口
func (s *RedisStore) StoreAgentJob(_ context.Context, job *livekit.Job) error {
	if job.Room == nil {
		return psrpc.NewErrorf(psrpc.InvalidArgument, "job doesn't have a valid Room field")
	}

	key := AgentJobPrefix + string(job.Room.Name)

	jb := utils.CloneProto(job)

	// Do not store room with the job
	jb.Room = nil

	// Only store the participant identity
	if jb.Participant != nil {
		jb.Participant = &livekit.ParticipantInfo{
			Identity: jb.Participant.Identity,
		}
	}

	data, err := proto.Marshal(jb)
	if err != nil {
		return err
	}

	return s.rc.HSet(s.ctx, key, job.Id, data).Err()
}

// DeleteAgentJob 删除AgentJob
// 实现了AgentStore接口
func (s *RedisStore) DeleteAgentJob(_ context.Context, job *livekit.Job) error {
	if job.Room == nil {
		return psrpc.NewErrorf(psrpc.InvalidArgument, "job doesn't have a valid Room field")
	}

	key := AgentJobPrefix + string(job.Room.Name)
	return s.rc.HDel(s.ctx, key, job.Id).Err()
}

// redisStoreOne 存储一个对象
func redisStoreOne(ctx context.Context, s *RedisStore, key, id string, p proto.Message) error {
	if id == "" {
		return errors.New("id is not set")
	}
	data, err := proto.Marshal(p)
	if err != nil {
		return err
	}
	return s.rc.HSet(s.ctx, key, id, data).Err()
}

// protoMsg 是proto.Message的类型约束
type protoMsg[T any] interface {
	*T
	proto.Message
}

// redisLoadOne 从Redis加载单个对象，返回指定类型的proto消息
func redisLoadOne[T any, P protoMsg[T]](ctx context.Context, s *RedisStore, key, id string, notFoundErr error) (P, error) {
	data, err := s.rc.HGet(s.ctx, key, id).Result()
	if err == redis.Nil {
		return nil, notFoundErr
	} else if err != nil {
		return nil, err
	}
	var p P = new(T)
	err = proto.Unmarshal([]byte(data), p)
	if err != nil {
		return nil, err
	}
	return p, err
}

// redisLoadAll 从Redis加载所有对象，返回指定类型的proto消息列表
func redisLoadAll[T any, P protoMsg[T]](ctx context.Context, s *RedisStore, key string) ([]P, error) {
	data, err := s.rc.HVals(s.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	list := make([]P, 0, len(data))
	for _, d := range data {
		var p P = new(T)
		if err = proto.Unmarshal([]byte(d), p); err != nil {
			return list, err
		}
		list = append(list, p)
	}

	return list, nil
}

// redisLoadBatch 从Redis按ID批量加载对象，返回指定类型的proto消息列表
// 如果keepEmpty为true，则保持返回结果的顺序与IDs列表一致，不存在的ID对应的位置返回零值
func redisLoadBatch[T any, P protoMsg[T]](ctx context.Context, s *RedisStore, key string, ids []string, keepEmpty bool) ([]P, error) {
	data, err := s.rc.HMGet(s.ctx, key, ids...).Result()
	if err == redis.Nil {
		if keepEmpty {
			return make([]P, len(ids)), nil
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if !keepEmpty {
		list := make([]P, 0, len(data))
		for _, v := range data {
			if d, ok := v.(string); ok {
				var p P = new(T)
				if err = proto.Unmarshal([]byte(d), p); err != nil {
					return list, err
				}
				list = append(list, p)
			}
		}
		return list, nil
	}
	// Keep zero values where ID was not found.
	list := make([]P, len(ids))
	for i := range ids {
		if d, ok := data[i].(string); ok {
			var p P = new(T)
			if err = proto.Unmarshal([]byte(d), p); err != nil {
				return list, err
			}
			list[i] = p
		}
	}
	return list, nil
}

// redisIDs 返回Redis哈希中的所有键，并按照字典顺序排序
func redisIDs(ctx context.Context, s *RedisStore, key string) ([]string, error) {
	list, err := s.rc.HKeys(s.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	slices.Sort(list)
	return list, nil
}

// protoEntity 是集成了ID()方法的proto消息接口，用于支持按ID进行排序和操作
type protoEntity[T any] interface {
	protoMsg[T]
	ID() string
}

// redisIterPage 从Redis中分页加载对象，支持分页偏移和数量限制
// 如果page.AfterId不为空，则从该ID之后开始查询
// 如果page.Limit大于0，则限制返回的最大数量
func redisIterPage[T any, P protoEntity[T]](ctx context.Context, s *RedisStore, key string, page *livekit.Pagination) ([]P, error) {
	if page == nil {
		return redisLoadAll[T, P](ctx, s, key)
	}
	ids, err := redisIDs(ctx, s, key)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if page.AfterId != "" {
		i, ok := slices.BinarySearch(ids, page.AfterId)
		if ok {
			i++
		}
		ids = ids[i:]
		if len(ids) == 0 {
			return nil, nil
		}
	}
	limit := 1000
	if page.Limit > 0 {
		limit = int(page.Limit)
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return redisLoadBatch[T, P](ctx, s, key, ids, false)
}

// sortProtos 对proto消息数组按照ID进行字典序排序
func sortProtos[T any, P protoEntity[T]](arr []P) {
	slices.SortFunc(arr, func(a, b P) int {
		return strings.Compare(a.ID(), b.ID())
	})
}

// sortPage 对proto消息数组按照ID进行排序，并根据分页参数限制返回结果数量
func sortPage[T any, P protoEntity[T]](items []P, page *livekit.Pagination) []P {
	sortProtos(items)
	if page != nil {
		if limit := int(page.Limit); limit > 0 && len(items) > limit {
			items = items[:limit]
		}
	}
	return items
}
