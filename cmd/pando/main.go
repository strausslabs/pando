package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/guyStrauss/pando/internal/cli"
)

var version = "dev"

func main() {
	err := cli.Execute(version)
	if err == nil {
		return
	}
	var handled cli.Handled
	if !errors.As(err, &handled) {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(1)
}
