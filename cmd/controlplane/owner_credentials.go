package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

const maxOwnerPasswordBytes = 1024

type ownerCredentials struct {
	Version int    `json:"version"`
	Email   string `json:"email"`
	Salt    []byte `json:"salt"`
	Hash    []byte `json:"password_hash"`
}

func normalizedOwnerEmail(value string) string {
	email := strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(email)
	if len(email) > 254 || err != nil || address.Name != "" || address.Address != email {
		return ""
	}
	return email
}

func ownerPasswordHash(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, 2, 19*1024, 1, 32)
}

func loadOwnerCredentials(path string) (*ownerCredentials, error) {
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("FORGEOS_OWNER_CREDENTIALS_FILE must be an absolute path")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not open FORGEOS_OWNER_CREDENTIALS_FILE")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return nil, errors.New("invalid Owner credentials file")
	}
	var credentials ownerCredentials
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil || decoder.Decode(new(any)) != io.EOF ||
		credentials.Version != 1 || credentials.Email == "" || normalizedOwnerEmail(credentials.Email) != credentials.Email ||
		len(credentials.Salt) != 16 || len(credentials.Hash) != 32 {
		return nil, errors.New("invalid Owner credentials file")
	}
	return &credentials, nil
}

func configureOwner(args []string) error {
	flags := flag.NewFlagSet("configure-owner", flag.ContinueOnError)
	path := flags.String("file", "", "absolute path to a new Owner credentials file")
	emailFlag := flags.String("email", "", "Owner email address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	email := normalizedOwnerEmail(*emailFlag)
	if !filepath.IsAbs(*path) || email == "" || flags.NArg() != 0 {
		return errors.New("usage: controlplane configure-owner -file <absolute new file path> -email <email address>")
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("run configure-owner in an interactive terminal to enter the password")
	}
	fmt.Fprint(os.Stderr, "Owner password (at least 12 characters): ")
	password, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return errors.New("could not read Owner password")
	}
	defer clear(password)
	if !utf8.Valid(password) || utf8.RuneCount(password) < 12 || len(password) > maxOwnerPasswordBytes {
		return errors.New("Owner password must contain at least 12 characters and at most 1024 UTF-8 bytes")
	}
	fmt.Fprint(os.Stderr, "Confirm Owner password: ")
	confirmation, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	defer clear(confirmation)
	if err != nil || !bytes.Equal(password, confirmation) {
		return errors.New("password confirmation did not match")
	}
	salt := make([]byte, 16)
	if _, err = rand.Read(salt); err != nil {
		return err
	}
	credentials := ownerCredentials{Version: 1, Email: email, Salt: salt, Hash: ownerPasswordHash(password, salt)}
	file, err := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(file).Encode(credentials)
	closeErr := file.Close()
	if err = errors.Join(encodeErr, closeErr); err != nil {
		_ = os.Remove(*path)
		return err
	}
	fmt.Fprintln(os.Stderr, "Owner credentials file created.")
	return nil
}
