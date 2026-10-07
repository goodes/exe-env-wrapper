package wrapper

import "strings"

// StripProgram removes the leading program name from a Windows command line,
// leaving the argument tail with its original quoting byte for byte. The first
// token follows the C runtime rule the loader itself uses: if it starts with a
// quote it runs to the next quote, otherwise to the first whitespace.
//
// It lives in a platform-independent file so it stays testable off Windows.
func StripProgram(cmdline string) string {
	i := 0
	if strings.HasPrefix(cmdline, `"`) {
		i = 1
		for i < len(cmdline) && cmdline[i] != '"' {
			i++
		}
		if i < len(cmdline) {
			i++ // the closing quote
		}
	} else {
		for i < len(cmdline) && !isCmdSpace(cmdline[i]) {
			i++
		}
	}
	return strings.TrimLeft(cmdline[i:], " \t")
}

func isCmdSpace(c byte) bool {
	return c == ' ' || c == '\t'
}
