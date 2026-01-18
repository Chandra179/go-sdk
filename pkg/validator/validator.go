package validator

import (
	"errors"
	"html"
	"regexp"
	"strings"
)

var (
	ProviderValidator  = regexp.MustCompile(`^(google|github)$`)
	TopicValidator     = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	ErrInvalidInput    = errors.New("invalid input")
	ErrMissingField    = errors.New("missing required field")
	ErrInvalidProvider = errors.New("invalid provider")
)

func ValidateProvider(provider string) error {
	if provider == "" {
		return ErrMissingField
	}
	if !ProviderValidator.MatchString(provider) {
		return ErrInvalidProvider
	}
	return nil
}

func ValidateTopic(topic string) error {
	if topic == "" {
		return ErrMissingField
	}
	if len(topic) > 249 {
		return ErrInvalidInput
	}
	if !TopicValidator.MatchString(topic) {
		return ErrInvalidInput
	}
	return nil
}

func SanitizeString(s string) string {
	return html.EscapeString(strings.TrimSpace(s))
}

func ValidateGroupID(groupID string) error {
	if groupID == "" {
		return ErrMissingField
	}
	if len(groupID) > 255 {
		return ErrInvalidInput
	}
	return nil
}
