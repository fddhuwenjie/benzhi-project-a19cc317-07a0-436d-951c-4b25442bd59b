package main

import (
	"fmt"
	"os"
)

func main() {
	c, err := parseConfig(os.Args[1:])
	if err == nil {
		if c.SelfCheck {
			err = runSelfCheck(c)
		} else {
			err = serveNormal(c)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
