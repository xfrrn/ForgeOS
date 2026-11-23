//go:build !windows

package main

import "io"

func showProjectPreviewProcess(_ io.Reader, _ io.Writer) {}
