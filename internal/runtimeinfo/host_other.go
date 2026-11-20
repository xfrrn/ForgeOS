//go:build !linux && !windows

package runtimeinfo

func hostResources() (*cpuTimes, *Space) { return nil, nil }
func diskSpace(string) *Space            { return nil }
