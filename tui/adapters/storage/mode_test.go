package storage

import (
	"os"
	"runtime"
)

func testMode(info os.FileInfo, want os.FileMode) bool {
	return runtime.GOOS == "windows" || info.Mode().Perm() == want
}
