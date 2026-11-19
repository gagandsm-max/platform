//go:build windows

// Standalone, non-production Windows filesystem capability probe.
// A successful call DOES NOT establish power-loss durability, namespace durability,
// or a permissible replacement for an existing queue fsync barrier.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

func printResult(name string, err error) {
	if err == nil {
		fmt.Printf("PROBE=%s RESULT=CALL_SUCCEEDED\n", name)
		return
	}
	if errno, ok := err.(syscall.Errno); ok {
		fmt.Printf("PROBE=%s RESULT=CALL_FAILED WIN32_ERROR=%d\n", name, uint32(errno))
		return
	}
	fmt.Printf("PROBE=%s RESULT=CALL_FAILED ERROR_CLASS=%T\n", name, err)
}

func probeDirectoryHandle(dir, label string, access uint32) {
	ptr, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		printResult(label+"_path", err)
		return
	}
	const share = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE
	h, err := syscall.CreateFile(ptr, access, share, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		printResult(label+"_open", err)
		return
	}
	defer syscall.CloseHandle(h)
	printResult(label+"_open", nil)
	printResult(label+"_flush", syscall.FlushFileBuffers(h))
}

func probeMoveWriteThrough(from, to string) {
	fromPtr, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		printResult("move_file_ex_path", err)
		return
	}
	toPtr, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		printResult("move_file_ex_path", err)
		return
	}
	const moveFileReplaceExisting = 0x1
	const moveFileWriteThrough = 0x8
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")
	if err := proc.Find(); err != nil {
		printResult("move_file_ex_find", err)
		return
	}
	result, _, err := proc.Call(
		uintptr(unsafe.Pointer(fromPtr)),
		uintptr(unsafe.Pointer(toPtr)),
		uintptr(moveFileReplaceExisting|moveFileWriteThrough),
	)
	runtime.KeepAlive(fromPtr)
	runtime.KeepAlive(toPtr)
	if result == 0 {
		printResult("move_file_ex_writethrough", err)
		return
	}
	printResult("move_file_ex_writethrough", nil)
}

func main() {
	fmt.Println("AMRUDAI_WINDOWS_QUEUE_DURABILITY_CAPABILITY_SPIKE_V1")
	fmt.Println("CLASSIFICATION=NONDESTRUCTIVE_TEMP_DIRECTORY_API_FEASIBILITY_ONLY")
	fmt.Printf("PLATFORM=%s/%s\n", runtime.GOOS, runtime.GOARCH)
	dir, err := os.MkdirTemp("", "amrudai-dir-sync-probe-")
	if err != nil {
		printResult("temp_directory_create", err)
		os.Exit(2)
	}
	defer os.RemoveAll(dir)
	fmt.Printf("TEMP_VOLUME=%s\n", filepath.VolumeName(dir))

	f, err := os.Open(dir)
	if err != nil {
		printResult("go_directory_open", err)
	} else {
		printResult("go_directory_open", nil)
		printResult("go_directory_sync", f.Sync())
		_ = f.Close()
	}

	probeDirectoryHandle(dir, "win32_dir_access_zero", 0)
	probeDirectoryHandle(dir, "win32_dir_write_attributes", syscall.FILE_WRITE_ATTRIBUTES)
	probeDirectoryHandle(dir, "win32_dir_generic_write", syscall.GENERIC_WRITE)

	from := filepath.Join(dir, "data-source.tmp")
	to := filepath.Join(dir, "data-target.dat")
	w, err := os.OpenFile(from, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		printResult("regular_file_create", err)
		os.Exit(3)
	}
	_, writeErr := w.Write([]byte("Windows queue capability probe; no production data.\n"))
	printResult("regular_file_write", writeErr)
	printResult("regular_file_sync", w.Sync())
	printResult("regular_file_close", w.Close())
	if writeErr == nil {
		probeMoveWriteThrough(from, to)
		if f, err := os.OpenFile(to, os.O_RDWR, 0); err == nil {
			printResult("renamed_file_sync", f.Sync())
			_ = f.Close()
		} else {
			printResult("renamed_file_open", err)
		}
	}
	fmt.Println("VERDICT=DIAGNOSTIC_ONLY_NOT_DURABILITY_CERTIFIED")
	fmt.Println("POWER_LOSS_SURVIVAL=NOT_TESTED")
	fmt.Println("NORMAL_WINDOWS_SERVICE_TOKEN=NOT_TESTED")
	fmt.Println("OTHER_FILESYSTEMS=NOT_TESTED")
}
