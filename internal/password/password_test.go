package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	password := "StrongPassword123!"

	hash, err := Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if hash == password {
		t.Fatal("password was stored as plaintext")
	}

	if !Verify(password, hash) {
		t.Fatal("Verify() rejected valid password")
	}

	if Verify("WrongPassword123!", hash) {
		t.Fatal("Verify() accepted invalid password")
	}
}

func TestHashProducesDifferentHashes(t *testing.T) {
	password := "StrongPassword123!"

	first, err := Hash(password)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Hash(password)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatal("password hashes are identical; salt is not random")
	}
}
