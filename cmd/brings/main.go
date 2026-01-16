package main

import (
	"os"

	"github.com/benithors/brings-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
