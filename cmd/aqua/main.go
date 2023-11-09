package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/aquaproj/aqua/v2/pkg/cli"
	"github.com/aquaproj/aqua/v2/pkg/log"
	"github.com/aquaproj/aqua/v2/pkg/runtime"
	"github.com/sirupsen/logrus"
	"github.com/suzuki-shunsuke/logrus-error/logerr"
)

var (
	version = ""
	commit  = "" //nolint:gochecknoglobals
	date    = "" //nolint:gochecknoglobals
)

type HasExitCode interface {
	ExitCode() int
}

func main() {
	// runtime.New 返回
	// &Runtime{
	//		GOOS:   goos(),
	//		GOARCH: goarch(),
	//	}
	rt := runtime.New()
	// 创建 logrus 实例
	logE := log.New(rt, version)
	// 启动 CLI
	if err := core(logE, rt); err != nil {
		var hasExitCode HasExitCode
		if errors.As(err, &hasExitCode) { // 是否有 EXIT_CODE
			code := hasExitCode.ExitCode()
			logerr.WithError(logE.WithField("exit_code", code), err).Debug("command failed")
			os.Exit(code)
		}
		logerr.WithError(logE, err).Fatal("aqua failed")
	}
}

func core(logE *logrus.Entry, rt *runtime.Runtime) error {
	runner := cli.Runner{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		LDFlags: &cli.LDFlags{
			Version: version,
			Commit:  commit,
			Date:    date,
		},
		LogE:    logE,
		Runtime: rt,
	}
	// 返回一个带 Done Channel 的 Ctx
	// Stop 函数用于释放资源？
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// 启动 CLI 程序
	// nolint 和 wrapcheck 是什么？
	return runner.Run(ctx, os.Args...) //nolint:wrapcheck
}
