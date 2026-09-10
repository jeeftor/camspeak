package util

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestCopyAt8kBpsPreservesFinalSamples(t *testing.T) {
	for _, size := range []int{1, 799, 801, 1599} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			input := bytes.Repeat([]byte{0xFF}, size)
			var output bytes.Buffer
			var mu sync.Mutex
			stopped := false
			if err := CopyAt8kBps(&output, bytes.NewReader(input), &stopped, &mu); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(input, output.Bytes()) {
				t.Fatalf("wrote %d of %d samples", output.Len(), size)
			}
		})
	}
}
