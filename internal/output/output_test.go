package output

import (
	"fmt"
	"io"
	"sync"
	"testing"
)

func TestServiceLog_ConcurrentServicesAreRaceSafe(t *testing.T) {
	oldOut, oldErr := stdout, stderr
	SetOutput(io.Discard, io.Discard)
	t.Cleanup(func() { SetOutput(oldOut, oldErr) })

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(service string) {
			defer wg.Done()
			ServiceLog(service, "ready")
		}(fmt.Sprintf("service-%d", i))
	}
	wg.Wait()
}
