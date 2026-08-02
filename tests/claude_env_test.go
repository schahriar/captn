package tests_test

import (
	"os"
	"slices"
	"testing"

	"github.com/schahriar/captn/pkg/providers"
	"github.com/stretchr/testify/assert"
)

func TestClaudeEnvStripsAPIKeyByDefault(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_BASE_URL", "https://example.test")

	env := providers.ClaudeEnv()

	assert.NotContains(t, env, "ANTHROPIC_API_KEY=sk-test")
	assert.Contains(t, env, "ANTHROPIC_BASE_URL=https://example.test")
	assert.Len(t, env, len(os.Environ())-1)
}

func TestClaudeEnvInheritsWhenUseAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	providers.UseAPIKey = true
	t.Cleanup(func() { providers.UseAPIKey = false })

	assert.Nil(t, providers.ClaudeEnv())
}

func TestClaudeEnvWithoutAPIKeySet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	assert.NoError(t, os.Unsetenv("ANTHROPIC_API_KEY"))

	env := providers.ClaudeEnv()

	assert.Equal(t, os.Environ(), env)
	assert.False(t, slices.ContainsFunc(env, func(kv string) bool {
		return kv == "ANTHROPIC_API_KEY=sk-test"
	}))
}
