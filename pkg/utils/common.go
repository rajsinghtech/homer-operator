package utils

// IsSubset checks if all key-value pairs in map2 exist in map1
// In other words, it checks if map2 is a subset of map1
func IsSubset(map1, map2 map[string]string) bool {
	for key, value := range map2 {
		if map1[key] != value {
			return false
		}
	}
	return true
}
