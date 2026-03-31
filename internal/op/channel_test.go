package op

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"github.com/stretchr/testify/assert"
)

func TestChannelGetOAuthByModel(t *testing.T) {
	// Save the original cache and restore it after the test
	originalCache := channelCache
	defer func() { channelCache = originalCache }()

	// Create a new test cache
	channelCache = cache.New[int, model.Channel](16)

	// Add test channels to the cache
	channelCache.Set(1, model.Channel{
		ID:       1,
		Name:     "Regular Channel",
		UseOAuth: false,
		Enabled:  true,
		Model:    "gpt-4",
	})

	channelCache.Set(2, model.Channel{
		ID:       2,
		Name:     "OAuth Channel 1",
		UseOAuth: true,
		Enabled:  true,
		Model:    "gpt-4,gpt-3.5-turbo",
	})

	channelCache.Set(3, model.Channel{
		ID:          3,
		Name:        "OAuth Channel 2",
		UseOAuth:    true,
		Enabled:     true,
		CustomModel: "claude-3,claude-2",
	})

	channelCache.Set(4, model.Channel{
		ID:       4,
		Name:     "Disabled OAuth Channel",
		UseOAuth: true,
		Enabled:  false,
		Model:    "disabled-model",
	})

	ctx := context.Background()

	tests := []struct {
		name      string
		modelName string
		wantID    int
		wantErr   bool
	}{
		{
			name:      "find model in Model field",
			modelName: "gpt-4",
			wantID:    2,
			wantErr:   false,
		},
		{
			name:      "find model in Model field with spaces",
			modelName: "gpt-3.5-turbo",
			wantID:    2,
			wantErr:   false,
		},
		{
			name:      "find model in CustomModel field",
			modelName: "claude-3",
			wantID:    3,
			wantErr:   false,
		},
		{
			name:      "model not found",
			modelName: "nonexistent-model",
			wantID:    0,
			wantErr:   true,
		},
		{
			name:      "disabled channel not returned",
			modelName: "disabled-model",
			wantID:    0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel, err := ChannelGetOAuthByModel(tt.modelName, ctx)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, channel)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, channel)
				assert.Equal(t, tt.wantID, channel.ID)
			}
		})
	}
}
