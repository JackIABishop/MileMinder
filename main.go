/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"os"
	"path/filepath"

	"github.com/jackbishop/mileage-cli/cmd"
)

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mileage-cli")
}

func main() {
	cmd.Execute()
}
