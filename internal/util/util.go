package util

func Coalesce[T comparable](l, r T) T {
	var zero T
	if l != zero {
		return l
	}
	return r
}
