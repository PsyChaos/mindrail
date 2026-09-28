package verify

import "testing"

func TestIsTestFileRecognizesTSX(t *testing.T) {
	for _, path := range []string{
		"apps/web/app/page.test.tsx",
		"apps/web/app/(console)/[org]/page.test.tsx",
	} {
		if !isTestFile(path) {
			t.Fatalf("TSX test file was skipped: %s", path)
		}
	}
}
