package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/myriadrf/limedrv"
	"github.com/racerxdl/lms_tcp/rtltcp"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var listenAddress = flag.String("a", "", "listen address")
var listenPort = flag.Int("p", 1234, "listen port (default: 1234)")
var gain = flag.Int("g", 0, "gain (default: 0 for auto)")
var sampleRate = flag.Int("s", 2048000, "samplerate in Hz (default: 2048000 Hz)")
var maxBuffers = flag.Int("b", -1, "number of buffers (kept for compatibility, ignored here)")
var deviceIndex = flag.Int("d", 0, "device index (default: 0)")
var oversampling = flag.Int("ov", 8, "oversampling (default: 8)")
var antennaName = flag.String("antenna", "LNAL", "antenna name (default: LNAL)")
var channel = flag.Int("channel", 0, "channel number (default: 0)")
var lpf = flag.Int("lpf", 2500000, "low pass filter (default 2500000)")

var server *rtltcp.Server

var tunerValues []int

func init() {
	for i := 0; i < 32; i++ {
		tunerValues = append(tunerValues, i*4)
	}
}

func OnSamples(samples []complex64, _ int, _ uint64) {
	if server != nil {
		server.ComplexBroadcast(samples)
	}
}

func main() {
	flag.Parse()
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

	dev.SetSampleRate(float64(*sampleRate), *oversampling)

	if len(dev.RXChannels) <= *channel {
		fmt.Printf("Invalid channel: %d. Found channels: \n", *channel)
		for i, v := range dev.RXChannels {
			fmt.Printf("	%d: %s\n", i, strings.ReplaceAll(v.String(), "\n", "\n\t"))
		}
		return
	}

	ch := dev.RXChannels[*channel]

	ch.Enable().
		SetAntennaByName(*antennaName).
		SetGainDB(uint(*gain)).
		SetLPF(float64(*lpf)).
		EnableLPF().
		SetCenterFrequency(106.3e6)

	dev.SetCallback(OnSamples)

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
			fmt.Printf("Setting sample rate to %d\n", sampleRate)
			dev.Stop()
			dev.SetSampleRate(float64(sampleRate), *oversampling)
			dev.Start()
			return true
		case rtltcp.SetAgcMode:
		case rtltcp.SetBiasTee:
		case rtltcp.SetGain:
			gain := binary.BigEndian.Uint32(cmd.Param[:])
			gainU := uint(gain / 10)
			fmt.Printf("Setting gain to %d\n", gainU)
			dev.SetGainDB(*channel, true, gainU)
		case rtltcp.SetGainMode:
		case rtltcp.SetTunerGainByIndex:
			gainIdx := binary.BigEndian.Uint32(cmd.Param[:])
			if uint32(len(tunerValues)) < gainIdx {
				gain := tunerValues[gainIdx]
				fmt.Printf("Setting gain to %d (idx %d)\n", gain, gainIdx)
				dev.SetGainDB(*channel, true, uint(gain))
			} else {
				fmt.Printf("Received gain index: %d but that's invalid. maximum is %d\n", len(tunerValues))
			}
		case rtltcp.SetFrequency:
			frequency := binary.BigEndian.Uint32(cmd.Param[:])
			fmt.Printf("Setting frequency to %d\n", frequency)
			dev.SetCenterFrequency(*channel, true, float64(frequency))
			return true
		default:
			fmt.Printf("Command %s not handled!\n", cmd.Type)
			return true
		}

		return true
	})
	server.SetOnConnect(func(sessionId string, address string) {
		fmt.Printf("New connection from %s [%s]\n", address, sessionId)
	})

	dev.Start()
	err := server.Start()
	if err != nil {
		fmt.Printf("Error starting rtltcp: %s\n", err)
		return
	}
	<-done
	server.Stop()
	dev.Stop()

	log.Println("Closed!")
}

/*
   -a listen address
   -p listen port (default: 1234)
   -f frequency to tune to [Hz]
   -g gain (default: 0 for auto)
   -s samplerate in Hz (default: 2048000 Hz)
   -b number of buffers (default: 32, set by library)
   -n max number of linked list buffers to keep (default: 500)
   -d device_index (default: 0)
*/
