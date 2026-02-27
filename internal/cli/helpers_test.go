package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveWorkingDir(t *testing.T) {
	t.Run("explicit dir is returned as-is", func(t *testing.T) {
		dir, err := resolveWorkingDir("/some/path")
		assert.NoError(t, err)
		assert.Equal(t, "/some/path", dir)
	})

	t.Run("empty dir returns cwd", func(t *testing.T) {
		dir, err := resolveWorkingDir("")
		assert.NoError(t, err)
		assert.NotEmpty(t, dir)
	})
}
