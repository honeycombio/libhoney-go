package marker

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const SecretENVVar = "API_SECRET_KEY"

func TestCreateMarker(t *testing.T) {
	apiKey := "ZiPnAPIuv2mOa33V4fsccA"
	client := NewClient(apiKey, USHost)
	err := client.CreateMarker("MyCodeExerciseDataset", "my message", "deploy")
	require.NoError(t, err)
}

func TestCreateMarker_InvalidDataset(t *testing.T) {
	apiKey := os.Getenv(SecretENVVar)
	client := NewClient(apiKey, USHost)
	err := client.CreateMarker("InvalidDataset", "my message", "test")
	require.Error(t, err)
}

func TestCreateMarker_InvalidAPIKey(t *testing.T) {
	client := NewClient("InvalidAPIKey", USHost)
	err := client.CreateMarker("MyCodeExerciseDataset", "my message", "test")
	require.Error(t, err)
}

// TODO: use the get all markers endpoint (https://docs.honeycomb.io/api/markers/list-all-markers) to validate the data saved to honeycomb
