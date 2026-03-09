package main

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/fang"
)

var Version = "dev"

func main() {
	cmd := newRootCmd()
	cmd.Version = Version

	if err := fang.Execute(context.Background(), cmd); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
