package opus

import "sync"

var (
	availOnce sync.Once
	avail     bool
)

// Available reports whether Opus encoding is supported in this binary (CGO + libopus).
func Available() bool {
	availOnce.Do(func() {
		_, err := NewEncoder(22050, 24000)
		avail = err == nil
	})
	return avail
}
