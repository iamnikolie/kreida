package share_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/internal/share"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) (share.Store, paths.Paths) {
	t.Helper()

	home := t.TempDir()
	p := paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}

	return share.Store{Paths: p}, p
}

func TestOneTokenPerBoardRevocable(t *testing.T) {
	s, _ := newStore(t)

	tok, err := s.Share("b1")
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9_-]{24}$`), tok)

	again, err := s.Share("b1")
	require.NoError(t, err)
	assert.Equal(t, tok, again)

	assert.Equal(t, "b1", s.BoardOf(tok))
	assert.Equal(t, "", s.BoardOf("../../etc"))

	n, err := s.Unshare("b1")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, "", s.BoardOf(tok))

	n, err = s.Unshare("b1")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestFileFormatAndMode(t *testing.T) {
	s, p := newStore(t)

	// an existing store: links listed in the order they were made
	old := "{\n  \"tokenAAAAAAAAAAAAAAAAAAAA\": {\n    \"board\": \"zeta\",\n    \"created\": \"2026-01-01T00:00:00.000Z\"\n  },\n" +
		"  \"tokenBBBBBBBBBBBBBBBBBBBB\": {\n    \"board\": \"alpha\",\n    \"created\": \"2026-01-02T00:00:00.000Z\"\n  }\n}\n"
	require.NoError(t, os.MkdirAll(p.Home, 0o755))
	require.NoError(t, os.WriteFile(p.SharesFile(), []byte(old), 0o600))

	links := s.All()
	require.Len(t, links, 2)
	assert.Equal(t, "zeta", links[0].Board, "creation order is kept, not sorted")
	assert.Equal(t, "alpha", s.BoardOf("tokenBBBBBBBBBBBBBBBBBBBB"))

	// rewriting without changes keeps the bytes identical
	_, err := s.Unshare("nobody")
	require.NoError(t, err)

	got, _ := os.ReadFile(p.SharesFile())
	assert.Equal(t, old, string(got))

	st, _ := os.Stat(p.SharesFile())
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
}

func TestFreshFileIsPrivate(t *testing.T) {
	s, p := newStore(t)

	_, err := s.Share("b")
	require.NoError(t, err)

	st, err := os.Stat(p.SharesFile())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())

	_, err = s.Unshare("b")
	require.NoError(t, err)

	got, _ := os.ReadFile(p.SharesFile())
	assert.Equal(t, "{}\n", string(got))
}

func TestTokenShapeIsChecked(t *testing.T) {
	s, _ := newStore(t)

	assert.Equal(t, "", s.BoardOf(""))
	assert.Equal(t, "", s.BoardOf("short"))
	assert.Equal(t, "", s.BoardOf("has spaces in it 1234567"))
}
