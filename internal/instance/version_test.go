package instance

import "testing"

func TestCheckVersionAcceptsOnlyThePinnedRelease(t *testing.T) {
	t.Cleanup(func(old string) func() { return func() { PinnedVersion = old } }(PinnedVersion))
	PinnedVersion = "0.47.0"
	for out, wantErr := range map[string]bool{
		`{"schema_version":1,"version":"0.47.0"}`:  false,
		`{"schema_version":1,"version":"v0.47.0"}`: false,
		`{"schema_version":2,"version":"0.47.0"}`:  true,
		`{"version":"0.47.0"}`:                     true,
		`{"schema_version":1,"version":"0.48.0"}`:  true,
		`{"schema_version":1,"version":"dev"}`:     true,
		`not json`:                                 true,
	} {
		if err := checkVersion([]byte(out)); (err != nil) != wantErr {
			t.Errorf("%s: err %v, want error %t", out, err, wantErr)
		}
	}
}
