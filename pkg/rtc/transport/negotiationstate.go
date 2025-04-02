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

import "fmt"

// NegotiationState 协商状态
type NegotiationState int

const (
	NegotiationStateNone NegotiationState = iota // 协商状态为空
	// waiting for remote description
	NegotiationStateRemote // 等待远程描述
	// need to Negotiate again
	NegotiationStateRetry // 需要重新协商
)

// String 返回协商状态的字符串表示
func (n NegotiationState) String() string {
	switch n {
	case NegotiationStateNone:
		return "NONE"
	case NegotiationStateRemote:
		return "WAITING_FOR_REMOTE"
	case NegotiationStateRetry:
		return "RETRY"
	default:
		return fmt.Sprintf("%d", int(n))
	}
}
