package vagrant

import (
	"testing"

	"github.com/quilt/quilt/db"
	"github.com/stretchr/testify/assert"
)

func TestSetACLs(t *testing.T) {
	prvdr := Provider{}
	assert.Nil(t, prvdr.SetACLs(nil))
}

func TestPreemptibleError(t *testing.T) {
	err := Provider{}.Boot([]db.Machine{{Preemptible: true}})
	assert.EqualError(t, err, "vagrant does not support preemptible instances")
}
