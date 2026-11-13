//go:build !race

package sftpfs_test

import "testing"

func skipUnderRace(*testing.T) {}
