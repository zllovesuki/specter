package chord

const (
	MaxFingerEntries = 16
)

func between(low, target, high uint64) bool {
	if target == high || (target > low && target <= high) {
		return true
	}
	return false
}
