package server

import "strings"

func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "Error 1146") ||
		strings.Contains(msg, "SQLSTATE 42S02")
}
