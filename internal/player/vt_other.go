//go:build !windows

package player

import "os"

func enableVT(*os.File) {}
