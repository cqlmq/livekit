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

package main

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/livekit/livekit-server/pkg/rtc"
	"github.com/livekit/livekit-server/pkg/telemetry/prometheus"
	"github.com/livekit/protocol/logger"

	"github.com/livekit/livekit-server/pkg/config"
	"github.com/livekit/livekit-server/pkg/routing"
	"github.com/livekit/livekit-server/pkg/service"
	"github.com/livekit/livekit-server/version"
)

// baseFlags 定义了基本的命令行参数
// Base command line flags for the server
var baseFlags = []cli.Flag{
	// 绑定地址参数：指定服务器要监听的IP地址
	// Bind addresses: specify IP addresses for the server to listen on
	&cli.StringSliceFlag{
		Name:  "bind",
		Usage: "IP address to listen on, use flag multiple times to specify multiple addresses",
	},

	// 配置文件路径参数
	// Path to configuration file
	&cli.StringFlag{
		Name:  "config",
		Usage: "path to LiveKit config file",
	},

	// YAML格式的配置内容，通常通过环境变量传入容器
	// Configuration content in YAML format, typically passed as environment variable in container
	&cli.StringFlag{
		Name:    "config-body",
		Usage:   "LiveKit config in YAML, typically passed in as an environment var in a container",
		EnvVars: []string{"LIVEKIT_CONFIG"},
	},

	// 密钥文件路径参数
	// Path to file that contains API keys/secrets
	&cli.StringFlag{
		Name:  "key-file",
		Usage: "path to file that contains API keys/secrets",
	},

	// API密钥参数
	// API keys (key: secret\n)
	&cli.StringFlag{
		Name:    "keys",
		Usage:   "api keys (key: secret)",
		EnvVars: []string{"LIVEKIT_KEYS"},
	},

	// 区域参数
	// Region of the current node. Used by regionaware node selector
	&cli.StringFlag{
		Name:    "region",
		Usage:   "region of the current node. Used by regionaware node selector",
		EnvVars: []string{"LIVEKIT_REGION"},
	},

	// 节点IP参数
	// IP address of the current node, used to advertise to clients. Automatically determined by default
	&cli.StringFlag{
		Name:    "node-ip",
		Usage:   "IP address of the current node, used to advertise to clients. Automatically determined by default",
		EnvVars: []string{"NODE_IP"},
	},

	// UDP端口参数
	// UDP port(s) to use for WebRTC traffic
	&cli.StringFlag{
		Name:    "udp-port",
		Usage:   "UDP port(s) to use for WebRTC traffic",
		EnvVars: []string{"UDP_PORT"},
	},

	// Redis主机参数
	// host (incl. port) to redis server
	&cli.StringFlag{
		Name:    "redis-host",
		Usage:   "host (incl. port) to redis server",
		EnvVars: []string{"REDIS_HOST"},
	},

	// Redis密码参数
	// password to redis
	&cli.StringFlag{
		Name:    "redis-password",
		Usage:   "password to redis",
		EnvVars: []string{"REDIS_PASSWORD"},
	},

	// TURN服务器证书文件参数
	// tls cert file for TURN server
	&cli.StringFlag{
		Name:    "turn-cert",
		Usage:   "tls cert file for TURN server",
		EnvVars: []string{"LIVEKIT_TURN_CERT"},
	},

	// TURN服务器密钥文件参数
	// tls key file for TURN server
	&cli.StringFlag{
		Name:    "turn-key",
		Usage:   "tls key file for TURN server",
		EnvVars: []string{"LIVEKIT_TURN_KEY"},
	},

	// 内存分析参数
	// Memory profile to write to `file`
	&cli.StringFlag{
		Name:  "memprofile",
		Usage: "write memory profile to `file`",
	},

	// goroutine分析参数
	// Goroutine profile to write to `file`
	&cli.StringFlag{
		Name:  "goroutineprofile",
		Usage: "write goroutine profile to `file` when receiving SIGUSR1",
	},

	// 开发模式参数
	// sets log-level to debug, console formatter, and /debug/pprof. insecure for production
	&cli.BoolFlag{
		Name:  "dev",
		Usage: "sets log-level to debug, console formatter, and /debug/pprof. insecure for production",
	},

	// 严格配置解析参数
	// disables strict config parsing
	&cli.BoolFlag{
		Name:   "disable-strict-config",
		Usage:  "disables strict config parsing",
		Hidden: true,
	},
}

// init 初始化函数，设置随机数种子
// Initialize function, sets random seed
func init() {
	rand.Seed(time.Now().Unix())
}

// main 程序入口函数
// Main entry point of the server
func main() {
	// 设置延迟执行的错误恢复函数
	// Set up deferred error recovery function
	defer func() {
		if rtc.Recover(logger.GetLogger()) != nil {
			os.Exit(1)
		}
	}()

	// 生成CLI标志（命令行参数）
	// Generate CLI flags
	generatedFlags, err := config.GenerateCLIFlags(baseFlags, true)
	if err != nil {
		fmt.Println(err)
	}

	// 创建CLI应用程序
	// Create CLI application
	app := &cli.App{
		Name:        "livekit-server",
		Usage:       "High performance WebRTC server",
		Description: "run without subcommands to start the server",
		Flags:       append(baseFlags, generatedFlags...),
		Action:      startServer, // 默认动作是启动服务器 / Default action is to start the server
		Commands: []*cli.Command{
			// 生成API密钥对
			// Generate API key pair
			{
				Name:   "generate-keys",
				Usage:  "generates an API key and secret pair",
				Action: generateKeys,
			},
			// 打印服务器配置的端口
			// Print ports that server is configured to use
			{
				Name:   "ports",
				Usage:  "print ports that server is configured to use",
				Action: printPorts,
			},
			{
				// this subcommand is deprecated, token generation is provided by CLI
				Name:   "create-join-token",
				Hidden: true,
				Usage:  "create a room join token for development use",
				Action: createToken,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "room",
						Usage:    "name of room to join",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "identity",
						Usage:    "identity of participant that holds the token",
						Required: true,
					},
					&cli.BoolFlag{
						Name:     "recorder",
						Usage:    "creates a hidden participant that can only subscribe",
						Required: false,
					},
				},
			},
			{
				Name:   "list-nodes",
				Usage:  "list all nodes",
				Action: listNodes,
			},
			{
				Name:   "help-verbose",
				Usage:  "prints app help, including all generated configuration flags",
				Action: helpVerbose,
			},
		},
		Version: version.Version,
	}

	// 运行应用程序
	// Run the application
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
	}
}

// getConfig 从命令行参数获取配置
// Get configuration from command line arguments
func getConfig(c *cli.Context) (*config.Config, error) {
	// 获取配置字符串
	// Get configuration string
	confString, err := getConfigString(c.String("config"), c.String("config-body"))
	if err != nil {
		return nil, err
	}

	// 设置配置解析模式
	// Set configuration parsing mode
	strictMode := true
	if c.Bool("disable-strict-config") {
		strictMode = false
	}

	// 创建新的配置对象
	// Create new configuration object
	conf, err := config.NewConfig(confString, strictMode, c, baseFlags)
	if err != nil {
		return nil, err
	}

	// 初始化日志配置
	// Initialize logger configuration
	config.InitLoggerFromConfig(&conf.Logging)

	// 开发模式特殊处理
	// Special handling for development mode
	if conf.Development {
		logger.Infow("starting in development mode")

		// 如果没有提供API密钥，使用默认密钥
		// Use default keys if no API keys provided
		if len(conf.Keys) == 0 {
			logger.Infow("no keys provided, using placeholder keys",
				"API Key", "devkey",
				"API Secret", "secret",
			)
			conf.Keys = map[string]string{
				"devkey": "secret",
			}
			shouldMatchRTCIP := false
			// when dev mode and using shared keys, we'll bind to localhost by default
			if conf.BindAddresses == nil {
				conf.BindAddresses = []string{
					"127.0.0.1",
					"::1",
				}
			} else {
				// 如果非回环地址被提供, 则我们匹配RTC IP到绑定地址
				// if non-loopback addresses are provided, then we'll match RTC IP to bind address
				// our IP discovery ignores loopback addresses
				for _, addr := range conf.BindAddresses {
					ip := net.ParseIP(addr)
					if ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
						shouldMatchRTCIP = true
					}
				}
			}
			if shouldMatchRTCIP {
				// 如果应该匹配RTC IP, 则我们添加绑定地址到RTC IPs
				for _, bindAddr := range conf.BindAddresses {
					conf.RTC.IPs.Includes = append(conf.RTC.IPs.Includes, bindAddr+"/24")
				}
			}
		}
	}
	return conf, nil
}

// startServer 启动服务器的主函数
// Main function to start the server
func startServer(c *cli.Context) error {
	// 获取配置
	// Get configuration
	conf, err := getConfig(c)
	if err != nil {
		return err
	}

	// 验证API密钥长度
	// Validate API key length
	err = conf.ValidateKeys()
	if err != nil {
		return err
	}

	if memProfile := c.String("memprofile"); memProfile != "" {
		if f, err := os.Create(memProfile); err != nil {
			return err
		} else {
			defer func() {
				// run memory profile at termination
				runtime.GC()
				_ = pprof.WriteHeapProfile(f)
				_ = f.Close()
			}()
		}
	}

	// 设置 goroutine profile
	// add -gcflags=-l=0 to compile flag
	// cqlmq 2025-03-09
	if goroutineProfile := c.String("goroutineprofile"); goroutineProfile != "" {
		go func() {
			sigusr1 := make(chan os.Signal, 1)
			signal.Notify(sigusr1, syscall.SIGUSR1)
			for range sigusr1 {
				f, err := os.Create(goroutineProfile)
				if err != nil {
					logger.Errorw("failed to create goroutine profile", err)
					continue
				}

				// 获取所有 goroutine 的堆栈跟踪
				prof := pprof.Lookup("goroutine")
				if prof == nil {
					logger.Errorw("could not find goroutine profile", errors.New("profile not found"))
					f.Close()
					continue
				}

				if err := prof.WriteTo(f, 0); err != nil {
					logger.Errorw("could not write profile", err)
				}
				f.Close()

				logger.Infow("wrote goroutine profile", "file", goroutineProfile)
			}
		}()
	}

	// 创建本地节点
	// Create local node
	// 数据样本：{
	//  "id":"ND_hp9Ny4sFYKBS",
	//  "ip":"113.206.32.164",
	//  "num_cpus":10,
	//  "stats":{"started_at":1741530520,"updated_at":1741530520},
	//  "state":1
	// }
	currentNode, err := routing.NewLocalNode(conf)
	if err != nil {
		return err
	}

	logger.Debugw("current node info", "id", currentNode.NodeID(), "ip", currentNode.NodeIP(), "type", currentNode.NodeType())

	// 初始化Prometheus监控
	// Initialize Prometheus monitoring
	if err := prometheus.Init(string(currentNode.NodeID()), currentNode.NodeType()); err != nil {
		return err
	}

	// 初始化服务器
	// Initialize server
	server, err := service.InitializeServer(conf, currentNode)
	if err != nil {
		logger.Warnw("initialize server", err)
		return err
	}

	// 设置信号处理
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	// 启动信号处理协程
	// Start signal handling goroutine
	go func() {
		// 第一次中断时优雅关闭，第二次中断时强制关闭
		// When the first interrupt occurs, gracefully shut down, and when the second interrupt occurs, forcefully shut down
		for i := 0; i < 2; i++ {
			sig := <-sigChan
			force := i > 0
			logger.Infow("exit requested, shutting down", "signal", sig, "force", force)
			go server.Stop(force)
		}
	}()

	// 启动服务器
	// Start the server
	return server.Start()
}

// getConfigString 获取配置字符串
// Get configuration string
// 从配置文件或配置体中获取配置字符串
// Get configuration string from config file or config body
// 参数优先级：配置体 > 配置文件
// Parameter priority: config body > config file
func getConfigString(configFile string, inConfigBody string) (string, error) {
	if inConfigBody != "" || configFile == "" {
		return inConfigBody, nil
	}

	outConfigBody, err := os.ReadFile(configFile)
	if err != nil {
		return "", err
	}

	return string(outConfigBody), nil
}
