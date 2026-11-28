// Command atlasctl materializa el contenido del arnés ATLAS en un proyecto.
package main

import (
	"os"

	"github.com/cedaniel200/atlasctl/internal/cli"
)

func main() {
	project, _ := os.Getwd()
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		Getenv:      os.Getenv,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Stdin:       os.Stdin,
		Interactive: isTerminal(os.Stdin),
		Project:     project,
	}))
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
