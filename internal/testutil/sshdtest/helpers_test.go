package sshdtest_test

import (
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func agentSigners(t *testing.T, c net.Conn) []ssh.Signer {
	t.Helper()
	s, err := agent.NewClient(c).Signers()
	if err != nil || len(s) != 1 {
		t.Fatalf("agent signers = %d, %v", len(s), err)
	}
	return s
}
