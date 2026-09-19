package contractstest_test

import (
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts/contractstest"
)

func TestReferenceStagingConformance(t *testing.T) {
	contractstest.RunStagingSuite(t, contractstest.ReferenceStaging)
}
