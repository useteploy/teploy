package main

import "github.com/useteploy/teploy/internal/cli"

var version = "dev"

func main() {
	cli.Execute(version)
}
