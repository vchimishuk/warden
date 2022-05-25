package slices

func Remove[E any](s []E, test func(e E) bool) []E {
	var r []E
	for _, e := range s {
		if !test(e) {
			r = append(r, e)
		}
	}

	return r
}

func Contains[E any](s []E, test func(e E) bool) bool {
	for _, e := range s {
		if test(e) {
			return true
		}
	}

	return false
}
