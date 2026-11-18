package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const supportsWriterProcessTree = true

// Give the native process the last job handle. Its exit also stops its tool descendants.
func trackWriterProcess(pid int) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return err
	}
	child, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_DUP_HANDLE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(child)
	if err := windows.AssignProcessToJobObject(job, child); err != nil {
		return err
	}
	var lifetime windows.Handle
	return windows.DuplicateHandle(windows.CurrentProcess(), job, child, &lifetime, 0, false, 0)
}

func writerTreeAlive(pid int) (bool, error) { return roleProcessAlive(pid) }
