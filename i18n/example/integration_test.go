package example

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	i18n "i18n"
)

var sharedTranslator i18n.Translator

func TestMain(m *testing.M) {
	// go test sets the working directory to the package directory,
	// so testdata/ is always relative to example/.
	cfg := i18n.Config{
		DefaultLocale:  "en",
		FallbackLocale: "en",
		MessageFiles: []string{
			"testdata/en.json",
			"testdata/es.json",
		},
		Format: "json",
	}

	var err error
	sharedTranslator, err = i18n.NewTranslator(cfg)
	if err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func newTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	return sharedTranslator
}

func TestTranslate_SimpleMessage(t *testing.T) {
	tr := newTranslator(t)
	msg, err := tr.T(context.Background(), "en", "greeting", nil, -1)
	require.NoError(t, err)
	assert.Equal(t, "Hello!", msg)
}

func TestTranslate_WithTemplateData(t *testing.T) {
	tr := newTranslator(t)
	msg, err := tr.T(context.Background(), "en", "welcome", map[string]any{"Name": "Alice"}, -1)
	require.NoError(t, err)
	assert.Equal(t, "Welcome, Alice!", msg)
}

func TestTranslate_Pluralisation(t *testing.T) {
	tr := newTranslator(t)
	ctx := context.Background()

	tests := []struct {
		name        string
		locale      string
		pluralCount int
		data        map[string]any
		expected    string
	}{
		{"en singular", "en", 1, map[string]any{"Count": 1}, "You have 1 item."},
		{"en plural", "en", 5, map[string]any{"Count": 5}, "You have 5 items."},
		{"es singular", "es", 1, map[string]any{"Count": 1}, "Tienes 1 artículo."},
		{"es plural", "es", 3, map[string]any{"Count": 3}, "Tienes 3 artículos."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := tr.T(ctx, tc.locale, "item_count", tc.data, tc.pluralCount)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, msg)
		})
	}
}

func TestTranslate_FallbackLocale(t *testing.T) {
	tr := newTranslator(t)
	msg, err := tr.T(context.Background(), "es", "welcome", map[string]any{"Name": "Bob"}, -1)
	require.NoError(t, err)
	assert.Equal(t, "¡Bienvenido, Bob!", msg)
}

func TestTranslate_MissingKey(t *testing.T) {
	tr := newTranslator(t)
	_, err := tr.T(context.Background(), "en", "nonexistent_key", nil, -1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, i18n.ErrMessageNotFound))
}

func TestTranslate_InvalidLocale(t *testing.T) {
	tr := newTranslator(t)
	_, err := tr.T(context.Background(), "invalid!!!locale", "greeting", nil, -1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, i18n.ErrLocaleNotFound))
}

func TestLocales(t *testing.T) {
	tr := newTranslator(t)
	assert.ElementsMatch(t, []string{"en", "es"}, tr.Locales())
}

func TestNewTranslator_MissingDefaultLocale(t *testing.T) {
	_, err := i18n.NewTranslator(i18n.Config{MessageFiles: []string{"testdata/en.json"}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, i18n.ErrBundleLoad))
}

func TestNewTranslator_MissingMessageFiles(t *testing.T) {
	_, err := i18n.NewTranslator(i18n.Config{DefaultLocale: "en"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, i18n.ErrBundleLoad))
}

func TestNewTranslator_NonexistentFile(t *testing.T) {
	_, err := i18n.NewTranslator(i18n.Config{
		DefaultLocale: "en",
		MessageFiles:  []string{"/nonexistent/en.json"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, i18n.ErrBundleLoad))
}
