package main

import (
	"os"

	"github.com/devcoons/dcalcon/cli/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args))
}
