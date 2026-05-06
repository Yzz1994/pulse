package ops

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// Run 处理运维子命令（log/status/start/stop/restart）
// service 为 "server" 或 "node"，对应 systemd unit pulse-server / pulse-node
func Run(service string, args []string) error {
	unit := "pulse-" + service

	switch args[0] {
	case "log":
		return runLog(unit, args[1:])
	case "status":
		return runSystemctl("status", unit)
	case "start":
		return runSystemctl("start", unit)
	case "stop":
		return runSystemctl("stop", unit)
	case "restart":
		return runSystemctl("restart", unit)
	default:
		return fmt.Errorf("未知子命令 %q，可用: log, status, start, stop, restart", args[0])
	}
}

func runLog(unit string, args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	noFollow := fs.Bool("no-follow", false, "不跟踪，仅输出历史日志后退出")
	lines := fs.Int("n", 50, "显示最近 N 行")
	if err := fs.Parse(args); err != nil {
		return err
	}

	jArgs := []string{"-u", unit, "-n", strconv.Itoa(*lines)}
	if !*noFollow {
		jArgs = append(jArgs, "-f")
	}

	cmd := exec.Command("journalctl", jArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func runSystemctl(action, unit string) error {
	cmd := exec.Command("systemctl", action, unit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// IsOpsCmd 判断第一个参数是否为运维子命令
func IsOpsCmd(arg string) bool {
	switch arg {
	case "log", "status", "start", "stop", "restart":
		return true
	}
	return false
}
