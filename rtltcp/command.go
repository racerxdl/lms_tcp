package rtltcp

type CommandType uint8

const (
	SetFrequency           CommandType = 0x01
	SetSampleRate          CommandType = 0x02
	SetGainMode            CommandType = 0x03
	SetGain                CommandType = 0x04
	SetFrequencyCorrection CommandType = 0x05
	SetIfStage             CommandType = 0x06
	SetTestMode            CommandType = 0x07
	SetAgcMode             CommandType = 0x08
	SetDirectSampling      CommandType = 0x09
	SetOffsetTuning        CommandType = 0x0A
	SetRtlCrystal          CommandType = 0x0B
	SetTunerCrystal        CommandType = 0x0C
	SetTunerGainByIndex    CommandType = 0x0D
	SetTunerBandwidth      CommandType = 0x0E
	SetBiasTee             CommandType = 0x0F
	Invalid                CommandType = 0xFF
)

func (c CommandType) String() string {
	return CommandTypeToName[c]
}

var CommandTypeToName = map[CommandType]string{
	SetFrequency:           "SetFrequency",
	SetSampleRate:          "SetSampleRate",
	SetGainMode:            "SetGainMode",
	SetGain:                "SetGain",
	SetFrequencyCorrection: "SetFrequencyCorrection",
	SetIfStage:             "SetIfStage",
	SetTestMode:            "SetTestMode",
	SetAgcMode:             "SetAgcMode",
	SetDirectSampling:      "SetDirectSampling",
	SetOffsetTuning:        "SetOffsetTuning",
	SetRtlCrystal:          "SetRtlCrystal",
	SetTunerCrystal:        "SetTunerCrystal",
	SetTunerGainByIndex:    "SetTunerGainByIndex",
	SetTunerBandwidth:      "SetTunerBandwidth",
	SetBiasTee:             "SetBiasTee",
}

type Command struct {
	Type  CommandType
	Param [4]byte
}
