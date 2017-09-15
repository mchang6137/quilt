package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quilt/quilt/db"
)

func TestGetContainer(t *testing.T) {
	t.Parallel()

	a := db.Container{StitchID: "4567"}
	b := db.Container{StitchID: "432"}

	res, err := GetContainer([]db.Container{a, b}, "4567")
	assert.Nil(t, err)
	assert.Equal(t, a, res)

	res, err = GetContainer([]db.Container{a, b}, "456")
	assert.Nil(t, err)
	assert.Equal(t, a, res)

	res, err = GetContainer([]db.Container{a, b}, "45")
	assert.Nil(t, err)
	assert.Equal(t, a, res)

	res, err = GetContainer([]db.Container{a, b}, "432")
	assert.Nil(t, err)
	assert.Equal(t, b, res)

	res, err = GetContainer([]db.Container{a, b}, "43")
	assert.Nil(t, err)
	assert.Equal(t, b, res)

	_, err = GetContainer([]db.Container{a, b}, "4")
	assert.EqualError(t, err, `ambiguous stitchIDs 4567 and 432`)

	_, err = GetContainer([]db.Container{a, b}, "1")
	assert.EqualError(t, err, `no container with stitchID "1"`)
}
