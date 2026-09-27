package agent

import (
	"os"
	"strings"
	"testing"
)

func TestJevRouteSourceIsCanonicalEmbeddedAsset(t *testing.T) {
	want, err := os.ReadFile("jev_route.py")
	if err != nil {
		t.Fatal(err)
	}
	if got := JevRouteSource(); got != string(want) {
		t.Fatal("embedded JEV adapter differs from canonical source")
	}
	if !strings.Contains(string(want), "https://api.typesafe.ai/v1/systemone") {
		t.Fatal("canonical JEV adapter is missing the TypeSafe endpoint")
	}
}
