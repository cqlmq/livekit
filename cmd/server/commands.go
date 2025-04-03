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

// Package main implements the LiveKit server command line interface
// 实现 LiveKit 服务器的命令行接口

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/utils"
	"github.com/livekit/protocol/utils/guid"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/pkg/service"
)

// generateKeys generates a new API key/secret pair for server authentication
// Used for creating new API credentials that clients can use to connect to LiveKit
// 生成新的 API 密钥/密钥对，用于服务器认证
// 用于创建新的 API 凭证，客户端可以使用这些凭证连接到 LiveKit
func generateKeys(_ *cli.Context) error {
	apiKey := guid.New(utils.APIKeyPrefix)
	secret := utils.RandomSecret()
	fmt.Println("API Key: ", apiKey)
	fmt.Println("API Secret: ", secret)
	return nil
}

// printPorts prints all ports that the server is configured to use
// This includes HTTP service ports, WebRTC ports (UDP/TCP), and TURN ports if enabled
// 打印服务器配置使用的所有端口
// 包括 HTTP 服务端口、WebRTC 端口(UDP/TCP)，以及启用时的 TURN 端口
func printPorts(c *cli.Context) error {
	conf, err := getConfig(c)
	if err != nil {
		return err
	}

	udpPorts := make([]string, 0)
	tcpPorts := make([]string, 0)

	tcpPorts = append(tcpPorts, fmt.Sprintf("%d - HTTP service", conf.Port))
	if conf.RTC.TCPPort != 0 {
		tcpPorts = append(tcpPorts, fmt.Sprintf("%d - ICE/TCP", conf.RTC.TCPPort))
	}
	if conf.RTC.UDPPort.Valid() {
		portStr, _ := conf.RTC.UDPPort.MarshalYAML()
		udpPorts = append(udpPorts, fmt.Sprintf("%s - ICE/UDP", portStr))
	} else {
		udpPorts = append(udpPorts, fmt.Sprintf("%d-%d - ICE/UDP range", conf.RTC.ICEPortRangeStart, conf.RTC.ICEPortRangeEnd))
	}

	if conf.TURN.Enabled {
		if conf.TURN.TLSPort > 0 {
			tcpPorts = append(tcpPorts, fmt.Sprintf("%d - TURN/TLS", conf.TURN.TLSPort))
		}
		if conf.TURN.UDPPort > 0 {
			udpPorts = append(udpPorts, fmt.Sprintf("%d - TURN/UDP", conf.TURN.UDPPort))
		}
	}

	fmt.Println("TCP Ports")
	for _, p := range tcpPorts {
		fmt.Println(p)
	}

	fmt.Println("UDP Ports")
	for _, p := range udpPorts {
		fmt.Println(p)
	}
	return nil
}

// helpVerbose prints detailed help information for the application
// Includes all configuration flags, both base and generated ones
// 打印应用程序的详细帮助信息
// 包括所有配置标志，包括基本标志和生成的标志
func helpVerbose(c *cli.Context) error {
	generatedFlags, err := config.GenerateCLIFlags(baseFlags, false)
	if err != nil {
		return err
	}

	c.App.Flags = append(baseFlags, generatedFlags...)
	return cli.ShowAppHelp(c)
}

// createToken creates a room access token for development purposes
// The token includes:
// - Room access permissions
// - User identity
// - Optional recorder permissions
// - 30-day validity period
//
// 创建用于开发目的的房间访问令牌
// 令牌包含：
// - 房间访问权限
// - 用户身份
// - 可选的录制权限
// - 30天的有效期
func createToken(c *cli.Context) error {
	room := c.String("room")
	identity := c.String("identity")

	conf, err := getConfig(c)
	if err != nil {
		return err
	}

	// use the first API key from config
	if len(conf.Keys) == 0 {
		// try to load from file
		if _, err := os.Stat(conf.KeyFile); err != nil {
			return err
		}
		f, err := os.Open(conf.KeyFile)
		if err != nil {
			return err
		}
		defer func() {
			_ = f.Close()
		}()
		decoder := yaml.NewDecoder(f)
		if err = decoder.Decode(conf.Keys); err != nil {
			return err
		}

		if len(conf.Keys) == 0 {
			return fmt.Errorf("keys are not configured")
		}
	}

	var apiKey string
	var apiSecret string
	for k, v := range conf.Keys {
		apiKey = k
		apiSecret = v
		break
	}

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     room,
	}
	if c.Bool("recorder") {
		grant.Hidden = true
		grant.Recorder = true
		grant.SetCanPublish(false)
		grant.SetCanPublishData(false)
	}

	at := auth.NewAccessToken(apiKey, apiSecret).
		AddGrant(grant).
		SetIdentity(identity).
		SetValidFor(30 * 24 * time.Hour)

	token, err := at.ToJWT()
	if err != nil {
		return err
	}

	fmt.Println("Token:", token)

	return nil
}

// listNodes displays information about all nodes in the LiveKit cluster
// Shows details such as:
// - Node ID and type
// - Number of rooms and participants
// - System stats (CPU, memory)
// - Network stats (bytes in/out)
//
// 显示 LiveKit 集群中所有节点的信息
// 显示的详细信息包括：
// - 节点 ID 和类型
// - 房间和参与者数量
// - 系统状态 (CPU、内存)
// - 网络状态 (出入流量)
func listNodes(c *cli.Context) error {
	conf, err := getConfig(c)
	if err != nil {
		return err
	}

	currentNode, err := routing.NewLocalNode(conf)
	if err != nil {
		return err
	}

	router, err := service.InitializeRouter(conf, currentNode)
	if err != nil {
		return err
	}

	nodes, err := router.ListNodes()
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{
		"ID", "IP Address", "Region",
		"CPUs", "CPU Usage\nLoad Avg",
		"Memory Used/Total",
		"Rooms", "Clients\nTracks In/Out",
		"Bytes/s In/Out\nBytes Total", "Packets/s In/Out\nPackets Total", "System Dropped Pkts/s\nPkts/s Out/Dropped",
		"Nack/s\nNack Total", "Retrans/s\nRetrans Total",
		"Started At\nUpdated At",
	})
	table.SetColumnAlignment([]int{
		tablewriter.ALIGN_CENTER, tablewriter.ALIGN_CENTER, tablewriter.ALIGN_CENTER,
		tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_CENTER,
	})

	for _, node := range nodes {
		stats := node.Stats
		rate := &livekit.NodeStatsRate{}
		if len(stats.Rates) > 0 {
			rate = stats.Rates[0]
		}

		// Id and state
		idAndState := fmt.Sprintf("%s\n(%s)", node.Id, node.State.Enum().String())

		// System stats
		cpus := strconv.Itoa(int(stats.NumCpus))
		cpuUsageAndLoadAvg := fmt.Sprintf("%.2f %%\n%.2f %.2f %.2f", stats.CpuLoad*100,
			stats.LoadAvgLast1Min, stats.LoadAvgLast5Min, stats.LoadAvgLast15Min)
		memUsage := fmt.Sprintf("%s / %s", humanize.Bytes(stats.MemoryUsed), humanize.Bytes(stats.MemoryTotal))

		// Room stats
		rooms := strconv.Itoa(int(stats.NumRooms))
		clientsAndTracks := fmt.Sprintf("%d\n%d / %d", stats.NumClients, stats.NumTracksIn, stats.NumTracksOut)

		// Packet stats
		bytes := fmt.Sprintf("%sps / %sps\n%s / %s", humanize.Bytes(uint64(rate.BytesIn)), humanize.Bytes(uint64(rate.BytesOut)),
			humanize.Bytes(stats.BytesIn), humanize.Bytes(stats.BytesOut))
		packets := fmt.Sprintf("%s / %s\n%s / %s", humanize.Comma(int64(rate.PacketsIn)), humanize.Comma(int64(rate.PacketsOut)),
			strings.TrimSpace(humanize.SIWithDigits(float64(stats.PacketsIn), 2, "")), strings.TrimSpace(humanize.SIWithDigits(float64(stats.PacketsOut), 2, "")))
		sysPacketsDroppedPct := float32(0)
		if rate.SysPacketsOut+rate.SysPacketsDropped > 0 {
			sysPacketsDroppedPct = float32(rate.SysPacketsDropped) / float32(rate.SysPacketsDropped+rate.SysPacketsOut)
		}
		sysPackets := fmt.Sprintf("%.2f %%\n%v / %v", sysPacketsDroppedPct*100, float64(rate.SysPacketsOut), float64(rate.SysPacketsDropped))
		nacks := fmt.Sprintf("%.2f\n%s", rate.NackTotal, strings.TrimSpace(humanize.SIWithDigits(float64(stats.NackTotal), 2, "")))
		retransmit := fmt.Sprintf("%.2f\n%s", rate.RetransmitPacketsOut, strings.TrimSpace(humanize.SIWithDigits(float64(stats.RetransmitPacketsOut), 2, "")))

		// Date
		startedAndUpdated := fmt.Sprintf("%s\n%s", time.Unix(stats.StartedAt, 0).UTC().UTC().Format("2006-01-02 15:04:05"),
			time.Unix(stats.UpdatedAt, 0).UTC().Format("2006-01-02 15:04:05"))

		table.Append([]string{
			idAndState, node.Ip, node.Region,
			cpus, cpuUsageAndLoadAvg,
			memUsage,
			rooms, clientsAndTracks,
			bytes, packets, sysPackets,
			nacks, retransmit,
			startedAndUpdated,
		})
	}
	table.Render()

	return nil
}
