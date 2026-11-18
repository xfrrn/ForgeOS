//go:build !linux

package main

import "context"

func (w *worker) syncResultWorktree(context.Context, string) error { return nil }
