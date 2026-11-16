package lockfile

import (
	"path/filepath"
	"strings"
)

// NUL cannot occur in an OS environment value, so a literal input cannot alias this hash-only marker.
const projectPathMarker = "\x00project\x00"

func normalizeCGOPaths(value, root string) string {
	if !strings.Contains(value, root) {
		return value
	}

	var output strings.Builder

	expectsPath := false

	for value != "" {
		prefix, word, rest := cgoWord(value)
		if word == "" && rest == "" {
			output.WriteString(prefix)

			break
		}

		if expectsPath || filepath.IsAbs(word) {
			word = normalizeCGOPath(word, root)
			expectsPath = false
		} else {
			word, expectsPath = normalizeCGOPathFlag(word, root)
		}

		output.WriteString(prefix)
		output.WriteString(word)

		if len(prefix) > 0 && (prefix[len(prefix)-1] == '\'' || prefix[len(prefix)-1] == '"') {
			output.WriteByte(rest[0])
			rest = rest[1:]
		}

		value = rest
	}

	return output.String()
}

func normalizeCGOPath(value, root string) string {
	if value == root {
		return projectPathMarker
	}

	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}

	if relative, inside := strings.CutPrefix(value, root); inside {
		return projectPathMarker + "/" + relative
	}

	return value
}

func normalizeCGOPathFlag(word, root string) (string, bool) {
	if value, found := strings.CutPrefix(word, "--sysroot="); found {
		return "--sysroot=" + normalizeCGOPath(value, root), false
	}

	for _, flag := range []string{
		"-iwithprefixbefore", "-iwithprefix", "-idirafter", "-isystem", "-isysroot", "-iquote",
		"-iprefix", "-include", "-imacros", "--sysroot", "-I", "-L", "-B",
	} {
		if word == flag {
			return word, true
		}

		if value, found := strings.CutPrefix(word, flag); found {
			return flag + normalizeCGOPath(value, root), false
		}
	}

	return word, false
}

// Go's quoted words allow quotes at the start of a word and do not unescape their contents.
func cgoWord(value string) (string, string, string) {
	const spaces = " \t\n\r"

	trimmed := strings.TrimLeft(value, spaces)

	start := len(value) - len(trimmed)
	if trimmed == "" {
		return value, "", ""
	}

	if quote := trimmed[0]; quote == '\'' || quote == '"' {
		end := strings.IndexByte(trimmed[1:], quote)
		if end < 0 {
			return value, "", ""
		}

		return value[:start+1], trimmed[1 : end+1], trimmed[end+1:]
	}

	end := strings.IndexAny(trimmed, spaces)
	if end < 0 {
		end = len(trimmed)
	}

	return value[:start], trimmed[:end], trimmed[end:]
}
