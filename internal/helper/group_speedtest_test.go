package helper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpeedTestAttemptContextTimeout(t *testing.T) {
	t.Parallel()

	start := time.Now()
	ctx, cancel := speedTestAttemptContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	assert.True(t, ok, "attempt context should have deadline")

	remaining := time.Until(deadline)
	assert.LessOrEqual(t, remaining, 10*time.Second)
	assert.Greater(t, remaining, 9*time.Second)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestSpeedTestAttemptContextRespectsParentDeadline(t *testing.T) {
	t.Parallel()

	parent, parentCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer parentCancel()

	ctx, cancel := speedTestAttemptContext(parent)
	defer cancel()

	deadline, ok := ctx.Deadline()
	assert.True(t, ok, "attempt context should have deadline")

	remaining := time.Until(deadline)
	assert.LessOrEqual(t, remaining, 2*time.Second)
	assert.Greater(t, remaining, 1*time.Second)
}
