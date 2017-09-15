package inspect

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quilt/quilt/stitch"
)

func TestStripExtension(t *testing.T) {
	test := map[string]string{
		"slug.blueprint":       "slug",
		"a/b/c/slug.blueprint": "a/b/c/slug",
		"foo":          "foo",
		"./foo/bar.js": "./foo/bar",
	}

	for inp, expect := range test {
		assert.Equal(t, expect, stripExtension(inp))
	}
}

// The expected graphviz graph returned by inspect when run on `testStitch`.
const expGraph = `strict digraph {
    "a";
    "b";
    "c";
    "public";

    "a" -> "b";
    "b" -> "c";
}`

func isGraphEqual(a, b string) bool {
	a = strings.Replace(a, "\n", "", -1)
	a = strings.Replace(a, " ", "", -1)
	b = strings.Replace(b, "\n", "", -1)
	b = strings.Replace(b, " ", "", -1)
	return a == b
}

func TestViz(t *testing.T) {
	t.Parallel()

	blueprint := stitch.Stitch{
		Containers: []stitch.Container{
			{
				Hostname: "a",
				ID:       "54be1283e837c6e40ac79709aca8cdb8ec5f31f5",
				Image:    stitch.Image{Name: "ubuntu"},
			},
			{
				Hostname: "b",
				ID:       "3c1a5738512a43c3122608ab32dbf9f84a14e5f9",
				Image:    stitch.Image{Name: "ubuntu"},
			},
			{
				Hostname: "c",
				ID:       "cb129f8a27df770b1dac70955c227a57bc5c4af6",
				Image:    stitch.Image{Name: "ubuntu"},
			},
		},
		Connections: []stitch.Connection{
			{From: "a", To: "b", MinPort: 22, MaxPort: 22},
			{From: "b", To: "c", MinPort: 22, MaxPort: 22},
		},
	}

	graph, err := New(blueprint)
	if err != nil {
		panic(err)
	}

	gv := makeGraphviz(graph)
	if !isGraphEqual(gv, expGraph) {
		t.Error(gv + "\n" + expGraph)
	}
}

func TestMainArgErr(t *testing.T) {
	t.Parallel()

	exitCode := Main([]string{"test.js"})
	assert.NotZero(t, exitCode)
}
