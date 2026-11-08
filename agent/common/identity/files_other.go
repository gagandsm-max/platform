//go:build !linux

package identity

import "os"

func validateIdentityAncestors(path string) error { return nil }

func publishIdentity(temp, target string) error { return os.Rename(temp, target) }

func openIdentityFile(path string) (*os.File, error) { return os.Open(path) }
func singleIdentityLink(info os.FileInfo) bool       { return true }

// Linux durability qualification does not establish Windows directory semantics.
func syncIdentityDir(path string) error { return nil }

func syncIdentityPath(path string, syncDir func(string) error) error { return syncDir(path) }
