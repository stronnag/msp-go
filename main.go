package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/xo/terminfo"
	"go.bug.st/serial/enumerator"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type SChan struct {
	len  uint16
	cmd  uint16
	ok   bool
	data []byte
}

func enumerate_ports() string {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		log.Fatal(err)
	}
	for _, port := range ports {
		if port.Name != "" {
			if port.IsUSB {
				if port.VID == "0483" && port.PID == "5740" ||
					port.VID == "0403" && port.PID == "6001" {
					return port.Name
				}
			}
		}
	}
	return ""
}

func main() {
	devnam := ""
	xsleep := false
	xonce := false
	mspvers := 2

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of mspview [options] device\n")
		flag.PrintDefaults()
	}

	flag.IntVar(&mspvers, "mspversion", 2, "MSP Version")
	flag.BoolVar(&xsleep, "slow", false, "Slow mode")
	flag.BoolVar(&xonce, "once", false, "Once only")
	flag.Parse()
	files := flag.Args()
	if len(files) > 0 {
		devnam = files[0]
	}

	if devnam == "" {
		devnam = "auto"
	}

	ti, err := terminfo.LoadFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		termreset(ti)
		if err != nil {
			log.Fatal(err)
		}
	}()

	terminit(ti)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	nmsg := 0

	var start time.Time
	var sp *MSPSerial
	c0 := make(chan SChan)

	upset := 3
	serok := false

	go func() {
		for {
			portnam := ""
			if devnam == "auto" {
				portnam = enumerate_ports()
			} else {
				portnam = devnam
			}
			if portnam != "" {
				sp, err = NewMSPSerial(portnam, c0, (mspvers == 2))
				if err == nil {
					fmt.Printf("Opened %s\n", portnam)
					nmsg = 0
					serok = true
					sp.MSPCommand(Msp_IDENT)
				} else {
					serok = false
				}
				for serok {
					select {
					case v := <-c0:
						nmsg += 1
						switch v.cmd {
						case Msp_IDENT:
							start = time.Now()
							if v.ok {
								fmt.Printf("MW Compat: %d, (msp protocol v%d)\n", v.data[0], mspvers)
							}
							sp.MSPCommand(Msp_NAME)
						case Msp_NAME:
							if v.ok && v.len > 0 {
								fmt.Printf("Name: \"%s\"\n", string(v.data))
							} else {
								fmt.Println("(noname)\n")
							}
							sp.MSPCommand(Msp_API_VERSION)
						case Msp_API_VERSION:
							if v.ok && v.len > 2 {
								fmt.Printf("API %d.%d\n", v.data[1], v.data[2])
							}
							sp.MSPCommand(Msp_FC_VARIANT)
						case Msp_FC_VARIANT:
							if v.ok {
								fmt.Printf("Variant: %s\n", string(v.data[0:4]))
							}
							sp.MSPCommand(Msp_FC_VERSION)
						case Msp_FC_VERSION:
							if v.ok {
								fmt.Printf("Version: %d.%d.%d\n", v.data[0], v.data[1], v.data[2])
							}
							sp.MSPCommand(Msp_BUILD_INFO)
						case Msp_BUILD_INFO:
							if v.ok {
								fmt.Printf("Build: %s %s (%s)\n", string(v.data[0:11]),
									string(v.data[11:19]), string(v.data[19:]))
							}
							sp.MSPCommand(Msp_BOARD_INFO)
						case Msp_BOARD_INFO:
							if v.ok {
								var board string
								if v.len > 8 {
									board = string(v.data[9:])
								} else {
									board = string(v.data[0:4])
								}
								fmt.Printf("Board: %s\n", board)
							}
							sp.MSPCommand(Msp_WP_GETINFO)
						case Msp_WP_GETINFO:
							if v.ok {
								wp_max := v.data[1]
								wp_valid := v.data[2]
								wp_count := v.data[3]
								fmt.Printf("WPINFO: %d of %d, valid %d\n", wp_count, wp_max, wp_valid)
							}
							if mspvers == 2 {
								sp.MSPCommand(Msp_MISC2)
							} else {
								sp.MSPCommand(Msp_ANALOG)
							}
						case Msp_MISC2:
							if v.ok {
								uptime := binary.LittleEndian.Uint32(v.data[0:4])
								fmt.Printf("MISC2: uptime %d", uptime)
								termclrLF(ti)
								upset = 4
							}
							sp.MSPCommand(Msp_ANALOG)
						case Msp_ANALOG:
							if v.ok {
								volts := float64(v.data[0]) / 10.0
								psum := binary.LittleEndian.Uint16(v.data[1:3])
								amps := float64(binary.LittleEndian.Uint16(v.data[5:7])) / 100.0
								fmt.Printf("ANA: v: %.1f, psum: %d, amps: %.2f", volts, psum, amps)
							} else {
								fmt.Printf("ANA: n/a")
							}
							termclrLF(ti)
							sp.MSPCommand(Msp_RAW_GPS)
						case Msp_RAW_GPS:
							if v.ok {
								fix := v.data[0]
								nsat := v.data[1]
								lat := float64(int32(binary.LittleEndian.Uint32(v.data[2:6]))) / 1e7
								lon := float64(int32(binary.LittleEndian.Uint32(v.data[6:10]))) / 1e7
								alt := int16(binary.LittleEndian.Uint16(v.data[10:12]))
								spd := float64(binary.LittleEndian.Uint16(v.data[12:14])) / 100.0
								cog := float64(binary.LittleEndian.Uint16(v.data[14:16])) / 10.0
								fmt.Printf("GPS: fix %d, sats %d,  %.6f° %.6f° %dm, spd %.1f cog %.0f", fix, nsat, lat, lon, alt, spd, cog)
								if len(v.data) > 16 {
									hdop := float64(binary.LittleEndian.Uint16(v.data[16:18])) / 100.0
									fmt.Printf("hdop %.1f", hdop)
								}
							} else {
								fmt.Printf("GPS: n/a")
							}
							termclrLF(ti)
							dura := time.Since(start).Seconds()
							rate := float64(nmsg) / dura
							fmt.Printf("\r%d messages in %.3fs (%.1f m/s)", nmsg, dura, rate)
							termclrLF(ti)
							termUp(ti, upset)
							if xonce {
								return
							}
							if xsleep {
								time.Sleep(time.Second * 1)
							}
							if mspvers == 2 {
								sp.MSPCommand(Msp_MISC2)
							} else {
								sp.MSPCommand(Msp_ANALOG)
							}
						case Msp_FAIL:
							serok = false
							sp = nil
						default:
							fmt.Printf("Unexpected MSP %v %d\n", v.ok, v.cmd)
						}
					}
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()
	<-c
	termout(ti)
}

func termUp(ti *terminfo.Terminfo, num int) {
	buf := new(bytes.Buffer)
	for j := 0; j < num; j++ {
		ti.Fprintf(buf, terminfo.CursorUp)
	}
	os.Stdout.Write(buf.Bytes())
}

func termclrLF(ti *terminfo.Terminfo) {
	buf := new(bytes.Buffer)
	ti.Fprintf(buf, terminfo.ClrEol)
	ti.Fprintf(buf, terminfo.Newline)
	os.Stdout.Write(buf.Bytes())
}

func termout(ti *terminfo.Terminfo) {
	buf := new(bytes.Buffer)
	ti.Fprintf(buf, terminfo.CursorDown)
	ti.Fprintf(buf, terminfo.CursorDown)
	ti.Fprintf(buf, terminfo.CursorDown)
	os.Stdout.Write(buf.Bytes())
}

func terminit(ti *terminfo.Terminfo) {
	buf := new(bytes.Buffer)
	ti.Fprintf(buf, terminfo.CursorInvisible)
	os.Stdout.Write(buf.Bytes())
}

func termreset(ti *terminfo.Terminfo) {
	buf := new(bytes.Buffer)
	ti.Fprintf(buf, terminfo.CursorNormal)
	os.Stdout.Write(buf.Bytes())
	fmt.Println("\n\n")
}
