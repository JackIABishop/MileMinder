/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"os"
	"path/filepath"

	"github.com/jackiabishop/mileminder/cmd"
)

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mileminder")
}

func main() {
	cmd.Execute()
}
