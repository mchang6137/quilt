package util

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToTar(t *testing.T) {
	content := fmt.Sprintf("a b c\neasy as\n1 2 3")
	out, err := ToTar("test_tar", 0644, content)

	if err != nil {
		t.Errorf("Error %#v while writing tar archive, expected nil", err.Error())
	}

	var buffOut bytes.Buffer
	writer := io.Writer(&buffOut)

	for tr := tar.NewReader(out); err != io.EOF; _, err = tr.Next() {
		if err != nil {
			t.Errorf("Error %#v while reading tar archive, expected nil",
				err.Error())
		}

		_, err = io.Copy(writer, tr)
		if err != nil {
			t.Errorf("Error %#v while reading tar archive, expected nil",
				err.Error())
		}
	}

	actual := buffOut.String()
	if actual != content {
		t.Error("Generated incorrect tar archive.")
	}
}

func TestWaitFor(t *testing.T) {
	var sleepCalls []time.Duration
	Sleep = func(t time.Duration) {
		sleepCalls = append(sleepCalls, t)
	}

	calls := 0
	callFourTimes := func() bool {
		calls++
		if calls == 4 {
			return true
		}
		return false
	}
	err := BackoffWaitFor(callFourTimes, 1*time.Second, 5*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, 4, calls, "predicate should be tested 4 times")
	assert.True(t, sleepCalls[1] > sleepCalls[0], "sleep interval should increase")
	assert.True(t, sleepCalls[2] > sleepCalls[1], "sleep interval should increase")

	err = BackoffWaitFor(func() bool {
		return false
	}, 1*time.Second, 300*time.Millisecond)
	assert.EqualError(t, err, "timed out")
}

func TestMapAsString(t *testing.T) {
	// Run the tests multiple times to test determinism.
	for i := 0; i < 10; i++ {
		assert.Equal(t, "[a=1 b=2]", MapAsString(
			map[string]string{"a": "1", "b": "2"}))

		// Nil and empty maps are the same.
		assert.Equal(t, "[]", MapAsString(nil))
		assert.Equal(t, "[]", MapAsString(map[string]string{}))
	}
}

func TestJoinNotifiers(t *testing.T) {
	t.Parallel()

	a := make(chan struct{})
	b := make(chan struct{})

	c := JoinNotifiers(a, b)

	timeout := time.Tick(30 * time.Second)

	select {
	case <-c:
	case <-timeout:
		t.FailNow()
	}

	a <- struct{}{}
	select {
	case <-c:
	case <-timeout:
		t.FailNow()
	}

	b <- struct{}{}
	select {
	case <-c:
	case <-timeout:
		t.FailNow()
	}

	a <- struct{}{}
	b <- struct{}{}
	select {
	case <-c:
	case <-timeout:
		t.FailNow()
	}
}
