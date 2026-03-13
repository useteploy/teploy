package main

import "github.com/teploy/teploy/internal/cli"

var version = "dev"

func main() {
	cli.Execute(version)
}
