package rtltcp

import "unsafe"

const DongleInfoSize = unsafe.Sizeof(DongleInfo{})

type DongleInfo struct {
	Magic          [4]uint8
	TunerType      TunerType
	TunerGainCount uint32
}
