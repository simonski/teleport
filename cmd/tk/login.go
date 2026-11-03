package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// loginInput reads a hidden terminal password or an explicitly supplied secret.
func loginInput(username string, passwordStdin bool, input *os.File, diagnostics io.Writer) (string, string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		username = strings.TrimSpace(os.Getenv("TELEPORT_USERNAME"))
	}
	if username == "" && passwordStdin {
		username = "admin"
	} // Preserve existing automation.
	if username == "" {
		if !term.IsTerminal(int(input.Fd())) {
			return "", "", errors.New("provide TELEPORT_USERNAME or -username, or sign in from a terminal")
		}
		fmt.Fprint(diagnostics, "Username: ")
		var name []byte
		for len(name) <= 1024 {
			var b [1]byte
			n, err := input.Read(b[:])
			if n > 0 {
				if b[0] == '\n' {
					break
				}
				name = append(name, b[0])
			}
			if err != nil {
				return "", "", errors.New("could not read username")
			}
		}
		username = strings.TrimSpace(string(name))
		if username == "" || len(name) > 1024 {
			return "", "", errors.New("provide a username within 1024 bytes")
		}
	}
	if len(username) > 1024 {
		return "", "", errors.New("username is too long")
	}
	var secret []byte
	var err error
	if passwordStdin {
		secret, err = io.ReadAll(io.LimitReader(input, 1026))
		if err != nil {
			return "", "", errors.New("could not read password")
		}
		secret = []byte(strings.TrimRight(string(secret), "\r\n"))
	} else if value, ok := os.LookupEnv("TELEPORT_PASSWORD"); ok {
		secret = []byte(value)
	} else {
		if !term.IsTerminal(int(input.Fd())) {
			return "", "", errors.New("provide TELEPORT_PASSWORD or use -password-stdin, or sign in from a terminal")
		}
		fmt.Fprint(diagnostics, "Password: ")
		secret, err = term.ReadPassword(int(input.Fd()))
		fmt.Fprintln(diagnostics)
		if err != nil {
			return "", "", errors.New("could not read password")
		}
	}
	if len(secret) == 0 || len(secret) > 1024 {
		return "", "", errors.New("password must be between 1 and 1024 bytes")
	}
	return username, string(secret), nil
}
