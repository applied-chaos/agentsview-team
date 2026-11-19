package secrets

import "testing"

func TestSealedValuesOpenOnlyWithTheSameKeyAndLabel(t *testing.T) {
	a, _ := NewBox("key-a")
	b, _ := NewBox("key-b")
	sealed, err := a.Seal("user-1/token", []byte("s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := a.Open("user-1/token", sealed); err != nil || string(got) != "s3cret" {
		t.Fatalf("round trip: %q %v", got, err)
	}
	if _, err := b.Open("user-1/token", sealed); err == nil {
		t.Fatal("opened with the wrong key")
	}
	if _, err := a.Open("user-2/token", sealed); err == nil {
		t.Fatal("opened under another label")
	}
}
