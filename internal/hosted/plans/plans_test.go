package plans_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/plans"
)

func TestPublishedPlanParity(t *testing.T) {
	expected, err := plans.JSON()
	if err != nil {
		t.Fatal(err)
	}
	published, err := os.ReadFile("../../../site/product/plans.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, published) {
		t.Fatal("public plan artifact differs from server; regenerate with serenity hosted plans")
	}
}
