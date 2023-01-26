package testcond

import (
	"fmt"
	"time"
)

// taken from https://yangwwei.github.io/2020/05/12/flaky-unit-tests-on-github.html
func WaitForCondition(eval func() bool, interval time.Duration, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if eval() {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for condition")
		}

		time.Sleep(interval)
	}
}
