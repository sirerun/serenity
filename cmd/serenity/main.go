package main

import (
	"fmt"
	"os"

	"github.com/sirerun/serenity/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
