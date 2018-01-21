package logging

import (
	"github.com/Sirupsen/logrus"
)

// PrefixedTextFormatter for logrus
type PrefixedTextFormatter struct {
	tfmt *logrus.TextFormatter

	prefix []byte
}

// New wraps logrus
func New() *logrus.Logger {
	return logrus.New()
}

// NewFormatter wraps logrus
func NewFormatter(prefix string) *PrefixedTextFormatter {
	return &PrefixedTextFormatter{
		tfmt:   &logrus.TextFormatter{},
		prefix: []byte(prefix),
	}
}

// Format for logrus
func (f *PrefixedTextFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	b, err := f.tfmt.Format(entry)
	if err != nil {
		return nil, err
	}
	return append(f.prefix, b...), nil
}
