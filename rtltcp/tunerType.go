package rtltcp

type TunerType uint32

const (
	RtlsdrTunerUnknown TunerType = iota
	RtlsdrTunerE4000
	RtlsdrTunerFc0012
	RtlsdrTunerFc0013
	RtlsdrTunerFc2580
	RtlsdrTunerR820t
	RtlsdrTunerR828d
)

var TunerTypeToName = map[TunerType]string{
	RtlsdrTunerUnknown: "Unknown",
	RtlsdrTunerE4000:   "E4000",
	RtlsdrTunerFc0012:  "FC0012",
	RtlsdrTunerFc0013:  "FC0013",
	RtlsdrTunerFc2580:  "FC2580",
	RtlsdrTunerR820t:   "R820T/2",
	RtlsdrTunerR828d:   "R828D",
}
