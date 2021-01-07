package main

import (
	"encoding/hex"
	"github.com/matiasinsaurralde/esp8266tool/fluepke-common"
	"github.com/mikepb/go-serial"
	"github.com/urfave/cli/v2"
	"log"
	"os"
	"time"
)

const (
	defaultBaudRate = 115200
	logPrefix       = "esp8266"

	mac0reg = 0x3ff00050
	mac1reg = 0x3ff00054
	mac3reg = 0x3ff0005c
)

var (
	portDevice string
)

// ESP8266 is the main data structure
type ESP8266 struct {
	portDevice string
	baudRate   int
	serialOpts serial.Options
	serialPort *serial.Port
	slipRW     *common.SlipReadWriter
	logger     *log.Logger
}

// NewESP8266 creates a new ESP8266 object
func NewESP8266(portDevice string, baudRate int) *ESP8266 {
	return &ESP8266{portDevice: portDevice, baudRate: baudRate}
}

// Connect sets the serial options and opens the port
func (e *ESP8266) Connect() (err error) {
	// Set default options:
	e.serialOpts = serial.RawOptions
	e.serialOpts.BitRate = e.baudRate
	e.serialOpts.Mode = serial.MODE_READ_WRITE
	e.serialPort, err = e.serialOpts.Open(e.portDevice)
	e.logger = log.New(os.Stdout, logPrefix, log.Ltime)
	e.slipRW = common.NewSlipReadWriter(e.serialPort, e.logger)
	return err
}

// Exec executes a command
func (e *ESP8266) Exec(cmd *common.Command) (*common.Response, error) {
	var done bool
	go func(cmd *common.Command) {
		for !done {
			e.slipRW.Write(cmd.ToBytes())
			time.Sleep(100 * time.Millisecond)
		}
	}(cmd)
	var rawData []byte
	var err error
	for !done {
		rawData, err = e.slipRW.Read(3 * time.Second)
		if err != nil {
			continue
		}
		if len(rawData) == 0 {
			continue
		}
		if rawData[1] != byte(cmd.Opcode) {
			continue
		}
		done = true
	}
	res, err := common.NewResponse(rawData)
	return res, err
}

// ReadRegister reads a register
func (e *ESP8266) ReadRegister(n uint32) (*common.Response, error) {
	cmd := common.NewReadRegisterCommand(n)
	return e.Exec(cmd)
}

// Sync executes the sync command as specified here: https://github.com/espressif/esptool/wiki/Serial-Protocol#initial-synchronisation
func (e *ESP8266) Sync() (*common.Response, error) {
	e.SetDTR(false)
	e.SetRTS(true)
	time.Sleep(100 * time.Millisecond)
	e.SetDTR(true)
	e.SetRTS(false)
	cmd := common.NewSyncCommand()
	time.Sleep(100 * time.Millisecond)
	return e.Exec(cmd)
}

// SetDTR sets DTR (Data Terminal Ready)
func (e *ESP8266) SetDTR(v bool) {
	defer e.serialPort.Apply(&e.serialOpts)
	if v {
		e.serialOpts.DTR = serial.DTR_ON
		return
	}
	e.serialOpts.DTR = serial.DTR_OFF
}

// SetRTS sets RTS (Ready To Send)
func (e *ESP8266) SetRTS(v bool) {
	defer e.serialPort.Apply(&e.serialOpts)
	if v {
		e.serialOpts.RTS = serial.RTS_ON
		return
	}
	e.serialOpts.RTS = serial.RTS_OFF
}

// ComputeMacAddr returns the device MAC address
func (e *ESP8266) ComputeMacAddr(mac0 [4]byte, mac1 [4]byte, mac3 [4]byte) (s string) {
	macAddr := []byte{
		mac3[2],
		mac3[1],
		mac3[0],
		mac1[1],
		mac1[0],
		mac0[3],
	}
	for i, b := range macAddr {
		if i >= 1 {
			s += ":"
		}
		s += hex.EncodeToString([]byte{b})
	}
	return
}

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "port",
				Usage:       "Serial port device",
				Destination: &portDevice,
				Required:    true,
			},
		},
		Name:  "read_mac",
		Usage: "Read MAC address from OTP ROM",
		Action: func(c *cli.Context) error {
			log.Println("Trying to establish connection with the device")
			esp := NewESP8266(portDevice, defaultBaudRate)
			err := esp.Connect()
			if err != nil {
				return err
			}
			log.Printf("Connection established with %s, baud rate is %d\n", esp.portDevice, esp.serialOpts.BitRate)
			_, err = esp.Sync()
			if err != nil {
				panic(err)
			}
			log.Println("SYNC ok")

			// Read registers:
			mac0, err := esp.ReadRegister(mac0reg)
			if err != nil {
				panic(err)
			}
			mac1, err := esp.ReadRegister(mac1reg)
			if err != nil {
				panic(err)
			}
			mac3, err := esp.ReadRegister(mac3reg)
			if err != nil {
				panic(err)
			}
			macAddr := esp.ComputeMacAddr(mac0.Value, mac1.Value, mac3.Value)
			log.Printf("MAC address is: %s\n", macAddr)
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
