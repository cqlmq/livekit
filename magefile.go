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

//go:build mage
// +build mage

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/magefile/mage/mg"

	"github.com/livekit/livekit-server/version"
	"github.com/livekit/mageutil"
	_ "github.com/livekit/psrpc"
)

const (
	goChecksumFile = ".checksumgo"            // 检查文件 用于检查文件是否被修改
	imageName      = "livekit/livekit-server" // 镜像名称
)

// Default target to run when none is specified
// If not set, running mage will list available targets
var (
	Default     = Build                                                       // 默认目标 构建
	checksummer = mageutil.NewChecksummer(".", goChecksumFile, ".go", ".mod") // 检查文件 用于检查文件是否被修改
)

func init() {
	checksummer.IgnoredPaths = []string{
		"pkg/service/wire_gen.go",  // 忽略wire_gen.go文件
		"pkg/rtc/types/typesfakes", // 忽略typesfakes文件 忽略测试假数据
	}
}

// 安装所有依赖
// explicitly reinstall all deps
func Deps() error {
	return installTools(true)
}

// builds LiveKit server
func Build() error {
	mg.Deps(generateWire)
	if !checksummer.IsChanged() {
		fmt.Println("up to date")
		return nil
	}

	fmt.Println("building...")
	if err := os.MkdirAll("bin", 0755); err != nil {
		return err
	}
	// 为了goroutineprofile加了编译参数：-gcflags=-l=0, 稳定后优化编译
	// if err := mageutil.RunDir(context.Background(), "cmd/server", "go build -o ../../bin/livekit-server"); err != nil {
	if err := mageutil.RunDir(context.Background(), "cmd/server", "go build -gcflags=-l=0 -o ../../bin/livekit-server"); err != nil {
		return err
	}

	checksummer.WriteChecksum()
	return nil
}

// builds binary that runs on linux amd64
func BuildLinux() error {
	mg.Deps(generateWire)
	if !checksummer.IsChanged() {
		fmt.Println("up to date")
		return nil
	}

	fmt.Println("building...")
	if err := os.MkdirAll("bin", 0755); err != nil {
		return err
	}
	cmd := mageutil.CommandDir(context.Background(), "cmd/server", "go build -buildvcs=false -o ../../bin/livekit-server-amd64")
	cmd.Env = []string{
		"GOOS=linux",
		"GOARCH=amd64",
		"HOME=" + os.Getenv("HOME"),
		"GOPATH=" + os.Getenv("GOPATH"),
	}
	if err := cmd.Run(); err != nil {
		return err
	}

	checksummer.WriteChecksum()
	return nil
}

func Deadlock() error {
	ctx := context.Background()
	if err := mageutil.InstallTool("golang.org/x/tools/cmd/goimports", "latest", false); err != nil {
		return err
	}
	if err := mageutil.Run(ctx, "go get github.com/sasha-s/go-deadlock"); err != nil {
		return err
	}
	if err := mageutil.Pipe("grep -rl sync.Mutex ./pkg", "xargs sed -i  -e s/sync.Mutex/deadlock.Mutex/g"); err != nil {
		return err
	}
	if err := mageutil.Pipe("grep -rl sync.RWMutex ./pkg", "xargs sed -i  -e s/sync.RWMutex/deadlock.RWMutex/g"); err != nil {
		return err
	}
	if err := mageutil.Pipe("grep -rl deadlock.Mutex\\|deadlock.RWMutex ./pkg", "xargs goimports -w"); err != nil {
		return err
	}
	if err := mageutil.Run(ctx, "go mod tidy"); err != nil {
		return err
	}
	return nil
}

func Sync() error {
	if err := mageutil.Pipe("grep -rl deadlock.Mutex ./pkg", "xargs sed -i  -e s/deadlock.Mutex/sync.Mutex/g"); err != nil {
		return err
	}
	if err := mageutil.Pipe("grep -rl deadlock.RWMutex ./pkg", "xargs sed -i  -e s/deadlock.RWMutex/sync.RWMutex/g"); err != nil {
		return err
	}
	if err := mageutil.Pipe("grep -rl sync.Mutex\\|sync.RWMutex ./pkg", "xargs goimports -w"); err != nil {
		return err
	}
	if err := mageutil.Run(context.Background(), "go mod tidy"); err != nil {
		return err
	}
	return nil
}

// builds and publish snapshot docker image
func PublishDocker() error {
	// don't publish snapshot versions as latest or minor version
	if !strings.Contains(version.Version, "SNAPSHOT") {
		return errors.New("Cannot publish non-snapshot versions")
	}

	versionImg := fmt.Sprintf("%s:v%s", imageName, version.Version)
	cmd := exec.Command("docker", "buildx", "build",
		"--push", "--platform", "linux/amd64,linux/arm64",
		"--tag", versionImg,
		".")
	mageutil.ConnectStd(cmd)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// run unit tests, skipping integration
func Test() error {
	mg.Deps(generateWire, setULimit)
	return mageutil.Run(context.Background(), "go test -short ./... -count=1")
}

// run all tests including integration
func TestAll() error {
	mg.Deps(generateWire, setULimit)
	return mageutil.Run(context.Background(), "go test ./... -count=1 -timeout=4m -v")
}

// cleans up builds
func Clean() {
	fmt.Println("cleaning...")
	os.RemoveAll("bin")
	os.Remove(goChecksumFile)
}

// regenerate code
func Generate() error {
	mg.Deps(installDeps, generateWire)

	fmt.Println("generating...")
	return mageutil.Run(context.Background(), "go generate ./...")
}

// 代码生成
// code generation for wiring
func generateWire() error {
	// 安装依赖
	mg.Deps(installDeps)

	fmt.Println("checksummer.IsChanged()", checksummer.IsChanged())

	// 检查文件是否被修改
	if !checksummer.IsChanged() {
		return nil
	}

	fmt.Println("wiring...")

	wire, err := mageutil.GetToolPath("wire")
	if err != nil {
		return err
	}

	fmt.Println("wire", wire)

	cmd := exec.Command(wire) // 准备wire命令
	cmd.Dir = "pkg/service"   // 指定目录
	mageutil.ConnectStd(cmd)  // 连接标准输出和标准错误输出

	// 执行wire命令 wire 在某目录下执行时，会自动寻找该目录下的wire.go文件 并生成wire_gen.go文件
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

// implicitly install deps
func installDeps() error {
	return installTools(false)
}

func installTools(force bool) error {
	tools := map[string]string{
		"github.com/google/wire/cmd/wire": "latest",
	}
	for t, v := range tools {
		if err := mageutil.InstallTool(t, v, force); err != nil {
			return err
		}
	}
	return nil
}
