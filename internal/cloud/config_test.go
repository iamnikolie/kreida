package cloud_test

import (
	"os"
	"testing"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveWritesPrivateFileAndLoadReadsIt(t *testing.T) {
	p := scratch(t)

	_, err := os.Stat(p.CloudFile())
	require.ErrorIs(t, err, os.ErrNotExist)

	c, err := cloud.Load(p)
	require.NoError(t, err)
	assert.Equal(t, cloud.Config{}, c, "no file: not logged in")
	assert.False(t, c.LoggedIn())

	require.NoError(t, cloud.Config{URL: "https://c.example", Token: token, Email: "me@example.com"}.Save(p))

	info, err := os.Stat(p.CloudFile())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	raw, err := os.ReadFile(p.CloudFile())
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"published": []`, "an empty list, not null")

	c, err = cloud.Load(p)
	require.NoError(t, err)
	assert.Equal(t, "https://c.example", c.URL)
	assert.Equal(t, token, c.Token)
	assert.Equal(t, "me@example.com", c.Email)
	assert.True(t, c.LoggedIn())
}

func TestSaveNarrowsAFileThatWasWider(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.WriteFile(p.CloudFile(), []byte(`{"token":"old"}`), 0o644))
	require.NoError(t, os.Chmod(p.CloudFile(), 0o644))

	require.NoError(t, cloud.Config{Token: token}.Save(p))

	info, err := os.Stat(p.CloudFile())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	entries, err := os.ReadDir(p.Home)
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".cloud-", "no temp file is left behind")
	}
}

func TestLoadNarrowsAFileThatIsWider(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.WriteFile(p.CloudFile(), []byte(`{"token":"`+token+`","published":["a"]}`), 0o644))
	require.NoError(t, os.Chmod(p.CloudFile(), 0o644))

	c, err := cloud.Load(p)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, c.Published)

	info, err := os.Stat(p.CloudFile())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadRejectsBrokenJSON(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.WriteFile(p.CloudFile(), []byte(`{nope`), 0o600))

	_, err := cloud.Load(p)
	assert.ErrorContains(t, err, "cloud.Load")
	assert.NotContains(t, err.Error(), "nope", "the file's text is not echoed")
}

func TestPublishedListIsSortedAndDeduplicated(t *testing.T) {
	p := scratch(t)

	c := cloud.Config{Token: token}.Add("b").Add("a").Add("b")
	assert.Equal(t, []string{"b", "a"}, c.Published, "Add keeps the order until saved")
	assert.True(t, c.Has("a"))

	require.NoError(t, c.Save(p))

	got, err := cloud.Load(p)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got.Published)

	d := got.Drop("a")
	assert.Equal(t, []string{"b"}, d.Published)
	assert.Equal(t, []string{"a", "b"}, got.Published, "Drop leaves the original alone")
	assert.False(t, d.Has("a"))
}

func TestLoginRequiresSavedEndpoint(t *testing.T) {
	t.Setenv(cloud.EnvURL, "https://env.example")
	assert.False(t, cloud.Config{Token: token}.LoggedIn())
	_, err := cloud.ClientFor(cloud.Config{Token: token})
	require.Error(t, err)

	cfg := cloud.Config{URL: "https://saved.example", Token: token}
	assert.True(t, cfg.LoggedIn())
	client, err := cloud.ClientFor(cfg)
	require.NoError(t, err)
	assert.Equal(t, cfg.URL, client.BaseURL)
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://kreida.example":          "https://kreida.example",
		"https://kreida.example/":         "https://kreida.example",
		" https://kreida.example/base// ": "https://kreida.example/base",
		"http://127.0.0.1:8080":           "http://127.0.0.1:8080",
		"http://localhost:8080/":          "http://localhost:8080",
		"http://[::1]:8080":               "http://[::1]:8080",
	} {
		got, err := cloud.NormalizeURL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}

	for _, in := range []string{
		"", "kreida.example", "ftp://kreida.example", "https://", "https://u:p@kreida.example",
		"https://kreida.example/?x=1", "http://kreida.example", "http://192.168.42.5:8080",
	} {
		_, err := cloud.NormalizeURL(in)
		assert.Error(t, err, in)
	}
}
