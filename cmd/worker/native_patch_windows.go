package main

import (
	"golang.org/x/sys/windows"
	"os"
)

func singlePatchLink(f *os.File, _ os.FileInfo) bool {
	var info windows.ByHandleFileInformation
	return windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info) == nil && info.NumberOfLinks == 1 && info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
