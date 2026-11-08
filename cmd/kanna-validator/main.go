// Command kanna-validator generates Validate methods from the validate tags of
// structs marked //kanna:validate.
package main

import (
	"os"

	"github.com/go-kanna/validator/internal/gen"
)

func main() {
	os.Exit(gen.Command().Main(os.Args[1:]))
}
