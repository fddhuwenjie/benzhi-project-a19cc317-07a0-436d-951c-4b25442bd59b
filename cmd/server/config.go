package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type config struct {
	Addr      string
	DataDir   string
	SelfCheck bool
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("cave-microclimate-clearance", flag.ContinueOnError)
	var c config
	fs.StringVar(&c.Addr, "addr", "", "完整回环监听地址")
	fs.StringVar(&c.DataDir, "data-dir", "data", "持久化数据目录")
	fs.BoolVar(&c.SelfCheck, "self-check", false, "执行真实 HTTP 全流程自检并退出")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 {
		return c, errors.New("不接受位置参数")
	}
	if c.Addr == "" {
		if raw := os.Getenv("PORT"); raw != "" {
			if strings.TrimSpace(raw) != raw {
				return c, errors.New("PORT 必须是纯端口号")
			}
			p, err := strconv.Atoi(raw)
			if err != nil || p < 1 || p > 65535 {
				return c, errors.New("PORT 必须是 1 到 65535 的纯端口号")
			}
			c.Addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(p))
		} else {
			c.Addr = "127.0.0.1:19081"
		}
	}
	if err := validateAddress(c.Addr); err != nil {
		return c, err
	}
	return c, nil
}

func validateAddress(addr string) error {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-addr 必须是 host:port: %w", err)
	}
	p, err := strconv.Atoi(portText)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("监听端口必须在 1 到 65535 之间")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("监听地址必须是明确的回环地址，禁止通配或外部地址")
	}
	return nil
}
