package main

import (
	"fmt"
	"log"
	"os"

	"pulse/internal/buildinfo"
	"pulse/internal/node"
	"pulse/internal/ops"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--version", "version":
			fmt.Println(buildinfo.Version)
			return
		case "-h", "--help", "help":
			printUsage()
			return
		}
		if ops.IsOpsCmd(os.Args[1]) {
			if err := ops.Run("node", os.Args[1:]); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	if err := node.Run(); err != nil {
		log.Fatal(err)
	}
}

func printUsage() {
	fmt.Println("用法:")
	fmt.Println("  pulse-node                   启动节点服务")
	fmt.Println("  pulse-node log               实时跟踪日志（默认 -f）")
	fmt.Println("  pulse-node log -n 200        显示最近 200 行")
	fmt.Println("  pulse-node log --no-follow   仅输出历史日志后退出")
	fmt.Println("  pulse-node status            查看服务状态")
	fmt.Println("  pulse-node start             启动服务（systemctl）")
	fmt.Println("  pulse-node stop              停止服务")
	fmt.Println("  pulse-node restart           重启服务")
	fmt.Println("  pulse-node version           查看版本")
}
