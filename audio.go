package main

func SetGain(s16_data []int16, amt float64) []int16 {
	for i := range s16_data {
		sample := float64(s16_data[i])
		sample *= amt
		if sample > 32767 {
			sample = 32767
		} else if sample < -32768 {
			sample = -32768
		}
		s16_data[i] = int16(sample)
	}
	return s16_data
}

