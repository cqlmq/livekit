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
	"net/http"
	"strings"

	"github.com/twitchtv/twirp"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
	accessTokenParam    = "access_token"
)

type grantsKey struct{}

type grantsValue struct {
	claims *auth.ClaimGrants
	apiKey string
}

var (
	// 权限拒绝错误
	ErrPermissionDenied = errors.New("permissions denied")
	// 缺少授权头错误
	ErrMissingAuthorization = errors.New("invalid authorization header. Must start with " + bearerPrefix)
	// 无效授权令牌错误
	ErrInvalidAuthorizationToken = errors.New("invalid authorization token")
	// 无效API密钥错误
	ErrInvalidAPIKey = errors.New("invalid API key")
)

// API密钥认证中间件
type APIKeyAuthMiddleware struct {
	provider auth.KeyProvider
}

// 创建API密钥认证中间件
func NewAPIKeyAuthMiddleware(provider auth.KeyProvider) *APIKeyAuthMiddleware {
	return &APIKeyAuthMiddleware{
		provider: provider,
	}
}

// 处理API密钥认证请求
func (m *APIKeyAuthMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// 如果请求路径是/rtc/validate，则设置CORS头
	if r.URL != nil && r.URL.Path == "/rtc/validate" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// 获取授权头：Authorization键的值
	authHeader := r.Header.Get(authorizationHeader)

	// 如果授权头不为空，则获取授权令牌
	var authToken string
	if authHeader != "" {
		// 如果授权头不以Bearer开头，则返回401错误
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			handleError(w, r, http.StatusUnauthorized, ErrMissingAuthorization)
			return
		}

		// 获取授权令牌
		authToken = authHeader[len(bearerPrefix):]
	} else {
		// attempt to find from request header
		// 如果请求头中没有Authorization键，则从请求参数中获取access_token
		authToken = r.FormValue(accessTokenParam)
	}

	if authToken != "" {
		v, err := auth.ParseAPIToken(authToken)
		if err != nil {
			handleError(w, r, http.StatusUnauthorized, ErrInvalidAuthorizationToken)
			return
		}

		secret := m.provider.GetSecret(v.APIKey())
		if secret == "" {
			handleError(w, r, http.StatusUnauthorized, errors.New("invalid API key: "+v.APIKey()))
			return
		}

		// 验证令牌，返回授权信息
		grants, err := v.Verify(secret)
		if err != nil {
			handleError(w, r, http.StatusUnauthorized, errors.New("invalid token: "+authToken+", error: "+err.Error()))
			return
		}

		// set grants in context
		ctx := r.Context()
		r = r.WithContext(context.WithValue(ctx, grantsKey{}, &grantsValue{
			claims: grants,
			apiKey: v.APIKey(),
		}))
	}

	next.ServeHTTP(w, r)
}

func WithAPIKey(ctx context.Context, grants *auth.ClaimGrants, apiKey string) context.Context {
	return context.WithValue(ctx, grantsKey{}, &grantsValue{
		claims: grants,
		apiKey: apiKey,
	})
}

func GetGrants(ctx context.Context) *auth.ClaimGrants {
	val := ctx.Value(grantsKey{})
	v, ok := val.(*grantsValue)
	if !ok {
		return nil
	}
	return v.claims
}

func GetAPIKey(ctx context.Context) string {
	val := ctx.Value(grantsKey{})
	v, ok := val.(*grantsValue)
	if !ok {
		return ""
	}
	return v.apiKey
}

func WithGrants(ctx context.Context, grants *auth.ClaimGrants, apiKey string) context.Context {
	return context.WithValue(ctx, grantsKey{}, &grantsValue{
		claims: grants,
		apiKey: apiKey,
	})
}

func SetAuthorizationToken(r *http.Request, token string) {
	r.Header.Set(authorizationHeader, bearerPrefix+token)
}

func EnsureJoinPermission(ctx context.Context) (name livekit.RoomName, err error) {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil {
		err = ErrPermissionDenied
		return
	}

	if claims.Video.RoomJoin {
		name = livekit.RoomName(claims.Video.Room)
	} else {
		err = ErrPermissionDenied
	}
	return
}

func EnsureAdminPermission(ctx context.Context, room livekit.RoomName) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil {
		return ErrPermissionDenied
	}

	if !claims.Video.RoomAdmin || room != livekit.RoomName(claims.Video.Room) {
		return ErrPermissionDenied
	}

	return nil
}

func EnsureCreatePermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.RoomCreate {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureListPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.RoomList {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureRecordPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.RoomRecord {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureIngressAdminPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil || !claims.Video.IngressAdmin {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureSIPAdminPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.SIP == nil || !claims.SIP.Admin {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureSIPCallPermission(ctx context.Context) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.SIP == nil || !claims.SIP.Call {
		return ErrPermissionDenied
	}
	return nil
}

func EnsureForwardPermission(ctx context.Context, source livekit.RoomName, destination livekit.RoomName) error {
	claims := GetGrants(ctx)
	if claims == nil || claims.Video == nil {
		return ErrPermissionDenied
	}

	if !claims.Video.RoomAdmin || source != livekit.RoomName(claims.Video.Room) || destination != livekit.RoomName(claims.Video.DestinationRoom) {
		return ErrPermissionDenied
	}

	return nil
}

// wraps authentication errors around Twirp
func twirpAuthError(err error) error {
	return twirp.NewError(twirp.Unauthenticated, err.Error())
}
