//go:build !windows && !linux

package runonce

func Lock() bool { return true }

func Unlock() {}
