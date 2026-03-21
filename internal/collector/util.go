package collector

import "strconv"

func itoa(n int) string { return strconv.Itoa(n) }

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
