package main

import (
	"fmt"
	"io"
	"strings"
)

// resolveSecretValueArg returns args[1] when it was given (and isn't the
// explicit "read from stdin" marker "-"); otherwise it reads the value from
// in. This keeps secret values out of shell history and process listings
// for the common "pipe it in" case, while still allowing a quick inline
// `acthur secrets set KEY value` for throwaway dev secrets.
func resolveSecretValueArg(args []string, in io.Reader) (string, error) {
	if len(args) == 2 && args[1] != "-" {
		return args[1], nil
	}
	data, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read secret value from stdin: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("no secret value provided — pass it as an argument or pipe it via stdin")
	}
	return value, nil
}
