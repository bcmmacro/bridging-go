package common

func IntSliceToMap(arr []int64) map[int64]bool {
	var ret = map[int64]bool{}
	for _, e := range arr {
		ret[e] = true
	}
	return ret
}

func Difference(s1, s2 map[int64]bool) []int64 {
	var ret []int64
	for k := range s1 {
		if _, ok := s2[k]; !ok {
			ret = append(ret, k)
		}
	}
	return ret
}
