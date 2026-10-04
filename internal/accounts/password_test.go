package accounts

import "testing"

func TestPasswordHash(t *testing.T) {
	encoded, err := hashPassword("a long secret password")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword("a long secret password", encoded) || checkPassword("wrong", encoded) {
		t.Fatal("password verification failed")
	}
	for _, bad := range []string{"", "pbkdf2-sha256-600000$00$00", "other$00$00"} {
		if checkPassword("a long secret password", bad) {
			t.Fatal("accepted malformed hash")
		}
	}
	other, _ := hashPassword("a long secret password")
	if encoded == other {
		t.Fatal("password salts are reused")
	}
}
