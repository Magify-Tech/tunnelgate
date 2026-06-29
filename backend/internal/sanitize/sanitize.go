package sanitize

import (
	"errors"
	"regexp"
	"strings"

	"postman-transform/backend-golang/internal/constants"
)

var (
	ErrUnsafeInput     = errors.New("contains unsafe input")
	controlChars       = regexp.MustCompile(constants.ControlCharsPattern)
	htmlTags           = regexp.MustCompile(constants.HTMLTagsPattern)
	highRiskSQLPattern = compilePatterns(constants.HighRiskSQLPatterns)
)

type Options struct {
	FieldName          string
	RejectSQLInjection bool
	StripHTMLTags      bool
}

func Text(value string, options Options) (string, error) {
	cleaned := controlChars.ReplaceAllString(value, "")
	if options.StripHTMLTags {
		cleaned = htmlTags.ReplaceAllString(cleaned, "")
	}

	if options.RejectSQLInjection {
		for _, pattern := range highRiskSQLPattern {
			if pattern.MatchString(cleaned) {
				return "", fieldError(options.FieldName)
			}
		}
	}
	return cleaned, nil
}

func Control(value, fieldName string) (string, error) {
	return Text(value, Options{FieldName: fieldName, RejectSQLInjection: true, StripHTMLTags: true})
}

func Payload(value, fieldName string) (string, error) {
	return Text(value, Options{FieldName: fieldName, RejectSQLInjection: false, StripHTMLTags: false})
}

func ControlArray(values []string, fieldName string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		cleaned, err := Control(value, fieldName)
		if err != nil {
			return nil, err
		}
		result = append(result, cleaned)
	}
	return result, nil
}

func fieldError(fieldName string) error {
	fieldName = strings.TrimSpace(fieldName)
	if fieldName == "" {
		fieldName = "Input"
	}
	return errors.New(fieldName + " " + ErrUnsafeInput.Error())
}

func compilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, regexp.MustCompile(pattern))
	}
	return compiled
}
