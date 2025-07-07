package utils

// IsSubset checks if the first map is a subset of the second map
func IsSubset(map1, map2 map[string]string) bool {
	for key, value := range map2 {
		if map1[key] != value {
			return false
		}
	}
	return true
}
