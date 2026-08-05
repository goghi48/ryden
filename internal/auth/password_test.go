package auth

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	t.Parallel()

	encoded, err := HashPassword("a sufficiently long password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	valid, err := VerifyPassword("a sufficiently long password", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !valid {
		t.Fatal("VerifyPassword() = false, want true")
	}
	valid, err = VerifyPassword("another password", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong) error = %v", err)
	}
	if valid {
		t.Fatal("VerifyPassword(wrong) = true, want false")
	}
}

func TestPasswordHashRejectsUnsafeParameters(t *testing.T) {
	t.Parallel()

	_, err := VerifyPassword(
		"password",
		"$argon2id$v=19$m=999999999,t=3,p=2$c2FsdA$a2V5",
	)
	if err == nil {
		t.Fatal("VerifyPassword() error = nil, want unsafe parameters error")
	}
}
