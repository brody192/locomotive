package logger

import (
	"log/slog"
	"strings"
)

func ErrAttr(err error) slog.Attr {
	if err == nil {
		return slog.String("err", "<nil>")
	}

	return slog.String("err", strings.TrimSpace(err.Error()))
}

func ErrorsAttr(errors ...error) slog.Attr {
	stringErrors := make([]string, 0, len(errors))

	for _, err := range errors {
		if err == nil {
			stringErrors = append(stringErrors, "<nil>")
			continue
		}

		stringErrors = append(stringErrors, strings.TrimSpace(err.Error()))
	}

	return slog.Any("errors", stringErrors)
}
