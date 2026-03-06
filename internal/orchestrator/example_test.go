package orchestrator_test

import (
	"fmt"
	"time"

	"github.com/bketelsen/gopilot/internal/orchestrator"
)

func ExampleBackoffDelay() {
	maxBackoff := 5 * time.Minute

	fmt.Println(orchestrator.BackoffDelay(1, maxBackoff))
	fmt.Println(orchestrator.BackoffDelay(2, maxBackoff))
	fmt.Println(orchestrator.BackoffDelay(3, maxBackoff))
	fmt.Println(orchestrator.BackoffDelay(5, maxBackoff))
	// Output:
	// 20s
	// 40s
	// 1m20s
	// 5m0s
}
