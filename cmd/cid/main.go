// Command cid switches between Claude Code accounts.
package main

import (
	"fmt"

	"github.com/nicoarbelaez/claude-identity/internal/version"
)

func main() {
	fmt.Printf("cid %s\n", version.String())
}
