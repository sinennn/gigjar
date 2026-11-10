package identity

import (
	"strings"
	"testing"
)

var fastParams = PasswordParams{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery", fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=1024,t=1,p=1$") {
		t.Errorf("unexpected encoding: %s", hash)
	}
	if ok, err := VerifyPassword("correct horse battery", hash); err != nil || !ok {
		t.Errorf("correct password rejected: %v", err)
	}
	if ok, _ := VerifyPassword("correct horse battery ", hash); ok {
		t.Error("wrong password accepted")
	}
}

func TestHashesAreSalted(t *testing.T) {
	first, _ := HashPassword("same password", fastParams)
	second, _ := HashPassword("same password", fastParams)
	if first == second {
		t.Error("identical hashes for identical passwords")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	for _, hash := range []string{"", "plaintext", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=18$m=1,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=19$m=1,t=1,p=1$!!$a2V5"} {
		if _, err := VerifyPassword("x", hash); err == nil {
			t.Errorf("accepted malformed hash %q", hash)
		}
	}
}
