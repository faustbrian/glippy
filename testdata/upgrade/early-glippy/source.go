package upgrade

func classify(first, second bool) int {
	if first {
		return 1
	} else if second {
		return 2
	} else if first {
		return 3
	}
	return 0
}
