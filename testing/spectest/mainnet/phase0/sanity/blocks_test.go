package sanity

import (
	"testing"

	"gitlab.waterfall.network/waterfall/protocol/coordinator/testing/spectest/shared/phase0/sanity"
)

func TestMainnet_Phase0_Sanity_Blocks(t *testing.T) {
	t.Skip() // Generate test data with pyton tool
	sanity.RunBlockProcessingTest(t, "test", "sanity/blocks/pyspec_tests")
}
