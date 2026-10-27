package main

import (
	"github.com/wsq2333/malta/internal/cli"
	"os"
)

func main() { os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr)) }
