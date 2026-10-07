// Command exe-env-wrapper sets the environment variables from a YAML file named
// after itself and then runs a target binary, passing everything through.
//
// It takes no flags of its own: every argument belongs to the target.
package main

import (
	"os"

	"github.com/goodes/exe-env-wrapper/internal/wrapper"
)

func main() {
	os.Exit(wrapper.Run())
}
