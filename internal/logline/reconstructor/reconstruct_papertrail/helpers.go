package reconstruct_papertrail

import "strings"

func getSeverityNumberFromSeverity(severity string) int {
	severity = strings.ToLower(severity)

	switch severity {
	case "debug":
		return 15 // facility 1 + severity 7
	case "info":
		return 14 // facility 1 + severity 6
	case "warn", "warning":
		return 12 // facility 1 + severity 4
	case "error", "err":
		return 11 // facility 1 + severity 3
	case "fatal", "critical":
		return 10 // facility 1 + severity 2
	case "alert":
		return 9 // facility 1 + severity 1
	case "emergency":
		return 8 // facility 1 + severity 0
	default:
		return 14 // default to info
	}
}

func getSeverityNumberFromStatusCode(statusCode int64) int {
	if statusCode >= 400 && statusCode <= 499 {
		return 12 // warning (facility 1 + severity 4)
	}

	if statusCode >= 500 && statusCode <= 599 {
		return 11 // error (facility 1 + severity 3)
	}

	return 14 // info (facility 1 + severity 6)
}
