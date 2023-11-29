package mwc

func splitSlice(input []byte, maxSize int) [][]byte {
	var result [][]byte

	for len(input) > 0 {
		size := len(input)
		if size > maxSize {
			size = maxSize
		}
		result = append(result, input[:size])
		input = input[size:]
	}

	return result
}
