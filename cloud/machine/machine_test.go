package machine

import (
	"testing"

	"github.com/quilt/quilt/blueprint"
)

func TestConstraints(t *testing.T) {
	checkConstraint := func(descriptions []Description, ram blueprint.Range,
		cpu blueprint.Range, maxPrice float64, exp string) {
		resSize := chooseBestSize(descriptions, ram, cpu, maxPrice)
		if resSize != exp {
			t.Errorf("bad size picked. Expected %s, got %s", exp, resSize)
		}
	}

	// Test all constraints specified with valid price
	testDescriptions := []Description{
		{Size: "size1", Price: 2, RAM: 2, CPU: 2},
	}
	checkConstraint(testDescriptions, blueprint.Range{Min: 1, Max: 3},
		blueprint.Range{Min: 1, Max: 3}, 2, "size1")

	// Test no max
	checkConstraint(testDescriptions, blueprint.Range{Min: 1},
		blueprint.Range{Min: 1}, 2, "size1")

	// Test exact match
	checkConstraint(testDescriptions, blueprint.Range{Min: 2},
		blueprint.Range{Min: 2}, 2, "size1")

	// Test no match
	checkConstraint(testDescriptions, blueprint.Range{Min: 3},
		blueprint.Range{Min: 2}, 2, "")

	// Test price too expensive
	checkConstraint(testDescriptions, blueprint.Range{Min: 2},
		blueprint.Range{Min: 2}, 1, "")

	// Test multiple matches (should pick cheapest)
	testDescriptions = []Description{
		{Size: "size2", Price: 2, RAM: 8, CPU: 4},
		{Size: "size3", Price: 1, RAM: 4, CPU: 4},
		{Size: "size4", Price: 0.5, RAM: 3, CPU: 4},
	}
	checkConstraint(testDescriptions, blueprint.Range{Min: 4},
		blueprint.Range{Min: 3}, 2, "size3")

	// Test infinite price
	checkConstraint(testDescriptions, blueprint.Range{Min: 4},
		blueprint.Range{Min: 3}, 0, "size3")

	// Test default ranges (should pick cheapest)
	checkConstraint(testDescriptions, blueprint.Range{},
		blueprint.Range{}, 0, "size4")

	// Test one default range (should pick only on the specified range)
	checkConstraint(testDescriptions, blueprint.Range{Min: 4},
		blueprint.Range{}, 0, "size3")
	checkConstraint(testDescriptions, blueprint.Range{Min: 3},
		blueprint.Range{}, 0, "size4")
}
