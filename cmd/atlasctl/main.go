// Command atlasctl materializa el contenido del arnés ATLAS en un proyecto.
package main

import (
	"os"

	"github.com/cedaniel200/atlasctl/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.Env{
		Getenv: os.Getenv,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}))
}
