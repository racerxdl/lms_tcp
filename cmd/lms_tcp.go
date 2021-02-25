package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/myriadrf/limedrv"
	"github.com/quan-to/slog"
	"github.com/racerxdl/lms_tcp/rtltcp"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var listenAddress = flag.String("a", "", "listen address")
var listenPort = flag.Int("p", 1234, "listen port (default: 1234)")
var gain = flag.Int("g", 0, "gain (default: 0 for auto)")
var sampleRate = flag.Int("s", 4000000, "sample rate in Hz (default: 2048000 Hz)")
var maxBuffers = flag.Int("b", -1, "number of buffers (kept for compatibility, ignored here)")
var deviceIndex = flag.Int("d", 0, "device index (default: 0)")
var oversampling = flag.Int("ov", 0, "oversampling (default: 0, maximum possible)")
var antennaName = flag.String("antenna", "LNAL", "antenna name (default: LNAL)")
var channel = flag.Int("channel", 0, "channel number (default: 0)")
var lpf = flag.Int("lpf", 2500000, "low pass filter (default 2500000)")
var verbose = flag.Bool("v", false, "verbose mode (default: off)")
var server *rtltcp.Server

var tunerValues []int

func init() {
	for i := 0; i < 32; i++ {
		tunerValues = append(tunerValues, int(float32(i)*2.5))
	}
}

func OnSamples16(samples []int16, _ int, _ uint64) {
	if server != nil {
		server.I16Broadcast(samples)
	}
}

func main() {
	flag.Parse()
	log := slog.Scope("LMSTCP")
	slog.SetDebug(*verbose)
	slog.SetShowLines(false)

	addr := fmt.Sprintf("%s:%d", *listenAddress, *listenPort)
	if *maxBuffers != -1 {
		fmt.Println("WARNING: number of buffers ignored on lms_tcp")
	}

	devices := limedrv.GetDevices()
	if len(devices) == 0 {
		panic("no devices found")
	}

	if len(devices) <= *deviceIndex || *deviceIndex < 0 {
		fmt.Printf("Invalid device index: %d. Found devices: \n", *deviceIndex)
		for i, v := range devices {
			fmt.Printf("	%d: %s [%s, %s]\n", i, v.DeviceName, v.Addr, v.Serial)
		}
		return
	}

	dev := limedrv.Open(devices[*deviceIndex])
	defer dev.Close()

	dev.SetI16CallbackMode(true)
	dev.SetI16Callback(OnSamples16)

	dev.SetSampleRate(float64(*sampleRate), *oversampling)

	if len(dev.RXChannels) <= *channel {
		log.Error("Invalid channel: %d. Found channels: ", *channel)
		for i, v := range dev.RXChannels {
			log.Error("	%d: %s", i, strings.ReplaceAll(v.String(), "\n", "\n\t"))
		}
		return
	}

	ch := dev.RXChannels[*channel]

	ch.Enable().
		SetAntennaByName(*antennaName).
		SetGainNormalized(1).
		SetLPF(float64(*lpf)).
		EnableLPF().
		SetCenterFrequency(106.3e6)

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		fmt.Println(sig)
		done <- true
	}()

	server = rtltcp.MakeRTLTCPServer(addr)
	server.SetDongleInfo(rtltcp.DongleInfo{
		TunerType:      rtltcp.RtlsdrTunerR820t,
		TunerGainCount: uint32(len(tunerValues)),
	})
	server.SetOnCommand(func(sessionId string, cmd rtltcp.Command) bool {
		switch cmd.Type {
		case rtltcp.SetSampleRate:
			sampleRate := binary.BigEndian.Uint32(cmd.Param[:])
			log.Info("Setting sample rate to %d with oversampling factor %d", sampleRate, *oversampling)
			dev.Stop()
			dev.SetSampleRate(float64(sampleRate), *oversampling)
			dev.Start()
			return true
		case rtltcp.SetAgcMode:
		case rtltcp.SetBiasTee:
		case rtltcp.SetGain:
			gain := binary.BigEndian.Uint32(cmd.Param[:])
			gainU := uint(gain / 4)
			log.Info("Setting gain to %d", gainU)
			dev.SetGainDB(*channel, true, gainU)
		case rtltcp.SetGainMode:
		case rtltcp.SetTunerGainByIndex:
			gainIdx := binary.BigEndian.Uint32(cmd.Param[:])
			if uint32(len(tunerValues)) > gainIdx {
				gain := tunerValues[gainIdx]
				log.Info("Setting gain to %d (idx %d)", gain, gainIdx)
				dev.SetGainDB(*channel, true, uint(gain))
			} else {
				log.Error("Received gain index: %d but that's invalid. maximum is %d", gainIdx, len(tunerValues))
			}
		case rtltcp.SetFrequency:
			frequency := binary.BigEndian.Uint32(cmd.Param[:])
			log.Info("Setting frequency to %d", frequency)
			dev.SetCenterFrequency(*channel, true, float64(frequency))
			return true
		default:
			log.Debug("Command %s not handled!", cmd.Type)
			return true
		}

		return true
	})
	server.SetOnConnect(func(sessionId string, address string) {
		log.Debug("New connection from %s [%s]", address, sessionId)
	})

	dev.Start()
	err := server.Start()
	if err != nil {
		log.Error("Error starting rtltcp: %s", err)
		return
	}
	log.Info("Setting gain to %d", *gain)
	ch.SetGainDB(uint(*gain))
	<-done
	server.Stop()
	dev.Stop()

	log.Info("Closed!")
}
