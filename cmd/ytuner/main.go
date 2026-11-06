// Command ytuner is the Go port of YTuner, a vTuner internet radio service
// replacement for AVRs (Yamaha, Denon, Marantz, Onkyo, Pioneer, ...).
package main

import (
	"os"

	"github.com/coffeegreg/ytuner-go/internal/ytuner"
)

func main() {
	os.Exit(ytuner.Run())
}
