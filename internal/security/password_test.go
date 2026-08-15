package security

import "testing"

func TestPasswordManagerHashAndCompare(t *testing.T) {
	t.Parallel()
	manager := PasswordManager{}
	password := "correct-horse-battery"

	hash, err := manager.Hash(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == password {
		t.Fatal("password was not hashed")
	}
	if err := manager.Compare(hash, password); err != nil {
		t.Fatalf("compare correct password: %v", err)
	}
	if err := manager.Compare(hash, "incorrect-password"); err == nil {
		t.Fatal("incorrect password unexpectedly matched")
	}
}
