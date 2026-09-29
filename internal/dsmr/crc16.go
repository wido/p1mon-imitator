package dsmr

// CRC16 computes the CRC-16/ARC checksum (polynomial 0xA001 reflected, init 0,
// no final XOR) that DSMR 4 and 5 meters append to every telegram.
func CRC16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
