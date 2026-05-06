package app

import (
	"fmt"
	"os"

	"pulse/internal/node"
	"pulse/internal/server"
)

func Run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "server":
		return server.Run()
	case "node":
		return node.Run()
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Fprintf(os.Stdout, "pulse — 代理节点控制面与节点管理系统。\n\n")
	fmt.Fprintf(os.Stdout, "用法:\n")
	fmt.Fprintf(os.Stdout, "  pulse server    启动控制面服务\n")
	fmt.Fprintf(os.Stdout, "  pulse node      启动节点服务\n")
	fmt.Fprintf(os.Stdout, "  pulse help      查看帮助\n")
}
