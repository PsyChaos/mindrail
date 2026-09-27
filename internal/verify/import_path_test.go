package verify

import (
	"path/filepath"
	"testing"

	"github.com/PsyChaos/mindrail/internal/testguard"
)

func TestImportTargetsFileResolvesLanguageRelativeParents(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name, baseline, language, module, target string
	}{
		{
			name: "python parent package", baseline: "pkg/tests/test_helper.py",
			language: testguard.LanguagePython, module: "..shared.helper", target: "pkg/shared/helper.py",
		},
		{
			name: "typescript parent directory", baseline: "pkg/tests/helper.test.ts",
			language: testguard.LanguageTypeScript, module: "../helper", target: "pkg/helper.ts",
		},
		{
			name: "javascript current directory", baseline: "pkg/tests/helper.test.js",
			language: testguard.LanguageJavaScript, module: "./support/helper.js", target: "pkg/tests/support/helper.js",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseline := filepath.Join(root, filepath.FromSlash(tc.baseline))
			if !importTargetsFile(root, baseline, tc.language, tc.module, true, tc.target) {
				t.Fatalf("%s from %s did not resolve to %s", tc.module, tc.baseline, tc.target)
			}
		})
	}
}

func TestImportTargetsFileRejectsDifferentRelativeTarget(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "pkg", "tests", "helper.test.ts")
	if importTargetsFile(root, baseline, testguard.LanguageTypeScript, "../helper", true, "other/helper.ts") {
		t.Fatal("relative import matched a different file")
	}
}
