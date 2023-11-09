package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aquaproj/aqua/v2/pkg/config"
	finder "github.com/aquaproj/aqua/v2/pkg/config-finder"
	"github.com/aquaproj/aqua/v2/pkg/log"
	"github.com/aquaproj/aqua/v2/pkg/policy"
	"github.com/aquaproj/aqua/v2/pkg/runtime"
	"github.com/sirupsen/logrus"
	"github.com/suzuki-shunsuke/go-osenv/osenv"
	"github.com/urfave/cli/v2"
)

type Runner struct {
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	LDFlags *LDFlags
	LogE    *logrus.Entry
	Runtime *runtime.Runtime
}

type LDFlags struct {
	Version string
	Commit  string
	Date    string
}

func (r *Runner) setParam(c *cli.Context, commandName string, param *config.Param) error { //nolint:funlen,cyclop
	param.Args = c.Args().Slice()
	if logLevel := c.String("log-level"); logLevel != "" {
		param.LogLevel = logLevel
	}
	param.ConfigFilePath = c.String("config")
	param.Dest = c.String("o")
	param.OutTestData = c.String("out-testdata")
	param.OnlyLink = c.Bool("only-link")
	if commandName == "generate-registry" {
		param.InsertFile = c.String("i")
	} else {
		param.Insert = c.Bool("i")
	}
	param.All = c.Bool("all")
	param.Detail = c.Bool("detail")
	param.Prune = c.Bool("prune")
	param.Limit = c.Int("limit")
	param.SelectVersion = c.Bool("select-version")
	param.File = c.String("f")
	if cmd := c.String("cmd"); cmd != "" {
		param.Commands = strings.Split(cmd, ",")
	}
	param.LogColor = os.Getenv("AQUA_LOG_COLOR")
	param.AQUAVersion = r.LDFlags.Version
	param.AquaCommitHash = r.LDFlags.Commit
	param.RootDir = config.GetRootDir(osenv.New())
	homeDir, _ := os.UserHomeDir()
	param.HomeDir = homeDir
	logE := r.LogE
	log.SetLevel(param.LogLevel, logE)
	log.SetColor(param.LogColor, logE)
	param.MaxParallelism = config.GetMaxParallelism(os.Getenv("AQUA_MAX_PARALLELISM"), logE)
	param.GlobalConfigFilePaths = finder.ParseGlobalConfigFilePaths(os.Getenv("AQUA_GLOBAL_CONFIG"))
	param.Deep = c.Bool("deep")
	param.Pin = c.Bool("pin")
	param.OnlyPackage = c.Bool("only-package")
	param.OnlyRegistry = c.Bool("only-registry")
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get the current directory: %w", err)
	}
	param.PWD = wd
	param.ProgressBar = os.Getenv("AQUA_PROGRESS_BAR") == "true"
	param.Tags = parseTags(strings.Split(c.String("tags"), ","))
	param.ExcludedTags = parseTags(strings.Split(c.String("exclude-tags"), ","))

	if a := os.Getenv("AQUA_DISABLE_LAZY_INSTALL"); a != "" {
		disableLazyInstall, err := strconv.ParseBool(a)
		if err != nil {
			return fmt.Errorf("parse the environment variable AQUA_DISABLE_LAZY_INSTALL as bool: %w", err)
		}
		param.DisableLazyInstall = disableLazyInstall
	}

	if a := os.Getenv("AQUA_DISABLE_POLICY"); a != "" {
		disablePolicy, err := strconv.ParseBool(a)
		if err != nil {
			return fmt.Errorf("parse the environment variable AQUA_DISABLE_POLICY as bool: %w", err)
		}
		param.DisablePolicy = disablePolicy
	}
	if !param.DisablePolicy {
		param.PolicyConfigFilePaths = policy.ParseEnv(os.Getenv("AQUA_POLICY_CONFIG"))
		for i, p := range param.PolicyConfigFilePaths {
			if !filepath.IsAbs(p) {
				param.PolicyConfigFilePaths[i] = filepath.Join(param.PWD, p)
			}
		}
	}
	if a := os.Getenv("AQUA_REQUIRE_CHECKSUM"); a != "" {
		requireChecksum, err := strconv.ParseBool(a)
		if err != nil {
			return fmt.Errorf("parse the environment variable AQUA_REQUIRE_CHECKSUM as bool: %w", err)
		}
		param.RequireChecksum = requireChecksum
	}
	return nil
}

func parseTags(tags []string) map[string]struct{} {
	tagsM := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		tagsM[tag] = struct{}{}
	}
	return tagsM
}

func (r *Runner) Run(ctx context.Context, args ...string) error {
	// 编译时间 ?
	compiledDate, err := time.Parse(time.RFC3339, r.LDFlags.Date)
	if err != nil {
		compiledDate = time.Now()
	}
	app := cli.App{
		Name:           "aqua",
		Usage:          "Version Manager of CLI. https://aquaproj.github.io/",
		Version:        r.LDFlags.Version + " (" + r.LDFlags.Commit + ")", // 版本号+Commit Hash
		Compiled:       compiledDate,                                      // 编译时间
		ExitErrHandler: exitErrHandlerFunc,
		Flags: []cli.Flag{ // CLI Flags
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "log level",
				EnvVars: []string{"AQUA_LOG_LEVEL"},
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "configuration file path",
				EnvVars: []string{"AQUA_CONFIG"},
			},
			&cli.StringFlag{
				Name:  "trace",
				Usage: "trace output file path",
			},
			&cli.StringFlag{
				Name:  "cpu-profile",
				Usage: "cpu profile output file path",
			},
		},
		EnableBashCompletion: true, // 开启 Bash 自动补全
		Commands: []*cli.Command{ // 子命令
			r.newInitCommand(),             // aqua init <filepath> 初始化配置文件，默认为 aqua.yaml
			r.newInfoCommand(),             // aqua info 获取当前 aqua 信息
			r.newInitPolicyCommand(),       // Deprecated
			r.newPolicyCommand(),           // aqua policy 是什么？
			r.newInstallCommand(),          // aqua install, i 安装 tools
			r.newUpdateAquaCommand(),       // aqua update-aqua, upa 更新 aqua
			r.newGenerateCommand(),         // aqua generate, g 搜索注册中心，查询可用的包
			r.newWhichCommand(),            // aqua which 输出指定 command 所在的文件位置
			r.newExecCommand(),             // aqua exec 执行指定 command
			r.newListCommand(),             // aqua list 列出注册中心的所有包
			r.newGenerateRegistryCommand(), // aqua generate-registry, gr 生成配置中心文件？
			r.newCompletionCommand(),       // aqua completion 生成自动补全脚本
			r.newVersionCommand(),          // aqua version 返回版本信息
			r.newCpCommand(),               // aqua cp 拷贝可执行文件
			r.newRootDirCommand(),          // aqua root-dir 输出 $AQUA_ROOT_DIR
			r.newUpdateChecksumCommand(),   // aqua update-checksum 更新或创建 aqua-checksums.json
			r.newRemoveCommand(),           // aqua remove, rm 卸载 package
			r.newUpdateCommand(),           // aqua update, up 更新注册中心和包
		},
	}
	// 启动 CLI App
	// 使用的 https://github.com/urfave/cli 框架构建
	return app.RunContext(ctx, args) //nolint:wrapcheck
}

func exitErrHandlerFunc(c *cli.Context, err error) {
	if c.Command.Name != "exec" {
		cli.HandleExitCoder(err)
		return
	}
}
