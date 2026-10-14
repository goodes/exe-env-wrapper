package wrapper

import "strings"

// versionStampPrefix makes the version findable inside a compiled binary.
//
// An installed wrapper is named after the tool it stands in for, so asking it
// for its version would pass "version" to that tool instead. The version has
// to be read out of the file, and a bare ldflags value — just "v0.3.3" — sits
// in the binary with nothing to search for. The prefix is the handle.
const versionStampPrefix = "exe-env-wrapper-version="

// VersionStamp is set at build time:
//
//	-ldflags "-X ...wrapper.VersionStamp=exe-env-wrapper-version=v1.2.3"
//
// It is the single stamped value; Version is derived from it, so the two
// cannot disagree. The initialiser has to stay a constant expression or the
// linker cannot patch it.
var VersionStamp = versionStampPrefix + "dev"

// Version is the build version, without the stamp's prefix.
var Version = strings.TrimPrefix(VersionStamp, versionStampPrefix)

// versionChars are the characters a stamped version may contain. A scan stops
// at the first byte outside this set, since what follows a string in a binary
// is whatever the linker happened to put next to it.
const versionChars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-+_"

// readVersionFromFile returns the version stamped into a wrapper binary.
//
// A wrapper built before the stamp existed reports false, and so does an
// unstamped development build: both are still wrappers, which isWrapperBinary
// answers separately, just ones whose version cannot be known.
func readVersionFromFile(path string) (string, bool) {
	tail, ok := scanFileFor(path, versionStampPrefix, looksLikeVersion)
	if !ok {
		return "", false
	}
	return leadingVersion(tail), true
}

// looksLikeVersion rejects a hit on the prefix that is not followed by a
// version.
//
// The prefix exists in the binary twice over: once as this very constant, and
// once as part of the stamped value. The constant's copy is followed by
// whatever string the linker placed after it, so hits have to be filtered
// rather than taken as the first match.
func looksLikeVersion(tail string) bool {
	version := leadingVersion(tail)
	switch {
	case version == "":
		return false
	case isDigit(version[0]):
		return true
	case version[0] == 'v' || version[0] == 'V':
		// "v1.2.3" counts; the default "dev" stamp of an unreleased build
		// does not, and reads as a version that cannot be known.
		return len(version) > 1 && isDigit(version[1])
	}
	return false
}

// leadingVersion is the version-shaped run at the start of s.
func leadingVersion(s string) string {
	rest := strings.TrimLeft(s, versionChars)
	return s[:len(s)-len(rest)]
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
